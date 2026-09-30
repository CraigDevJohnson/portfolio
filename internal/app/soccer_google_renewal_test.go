package app

import (
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	internalgoogle "portfolio/internal/google"
)

// These tests drive Add and result Sync (#93) through the real route assembly
// when the connection's access token is due for renewal and Google, or the
// connection store, cannot renew it. Only Google rejecting the grant itself
// ends the connection; any other failure keeps it and its chosen calendar.

// googleWriteAction is an action that writes to the chosen Google calendar
// for games in the fetched schedule.
type googleWriteAction struct {
	name string
	run  func(t *testing.T, world *calendarDestinationWorld) string
	// done is part of what the action reports once it reached the calendar.
	done string
}

var googleWriteActions = []googleWriteAction{
	{
		name: "Add",
		run:  func(t *testing.T, world *calendarDestinationWorld) string { return world.add(t, nextGameID) },
		done: "Added 1 selected game",
	},
	{
		name: "Sync",
		run: func(t *testing.T, world *calendarDestinationWorld) string {
			return world.syncResults(t, scoredPastGameID)
		},
		done: "game result(s) updated in Google Calendar",
	},
}

// syncResults asks the site to sync the selected past games' results and
// returns the route's response.
func (world *calendarDestinationWorld) syncResults(t *testing.T, gameIDs ...string) string {
	t.Helper()
	synced := world.browser.postForm("/soccer/google/sync-results", url.Values{"team_codes": {destinationTeamID}, "selected": gameIDs})
	if synced.Code != http.StatusOK {
		t.Fatalf("result sync status = %d", synced.Code)
	}
	return synced.Body.String()
}

// holdsConnectionCookie reports whether the browser still holds the site
// owner's Google connection cookie.
func (world *calendarDestinationWorld) holdsConnectionCookie() bool {
	return world.browser.holdsCookie(internalgoogle.ConnectionCookieName(world.cognito.issuer, world.cognito.subject), "/soccer")
}

// connectWithDueToken connects the fake Google account with access tokens
// that are due for renewal at each use, chooses the team calendar as the
// destination, and fetches the team's schedule.
func (world *calendarDestinationWorld) connectWithDueToken(t *testing.T) {
	t.Helper()
	world.google.issueShortLivedTokens()
	world.connect(t)
	world.choose(t, teamCalendarID)
	world.fetch(t)
	if len(world.store.snapshot()) != 1 || !world.holdsConnectionCookie() {
		t.Fatal("consent did not leave a stored connection and its cookie")
	}
}

// googleTokenAnswer answers a token request the way Google's token endpoint
// does, with status and a JSON body.
func googleTokenAnswer(status int, body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// dropGoogleConnection closes the connection without answering, as a
// network failure between the site and Google does.
func dropGoogleConnection(w http.ResponseWriter) {
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		panic("the fake Google server cannot drop a connection")
	}
	if conn, _, err := hijacker.Hijack(); err == nil {
		_ = conn.Close()
	}
}

func TestATemporaryFailureToRenewGoogleAccessKeepsTheConnectionAndItsCalendar(t *testing.T) {
	for _, failure := range []struct {
		name  string
		start func(world *calendarDestinationWorld)
	}{
		{name: "Google token endpoint unavailable", start: func(world *calendarDestinationWorld) {
			world.google.answerRenewals(googleTokenAnswer(http.StatusServiceUnavailable, `{"error":"internal_failure","error_description":"Service unavailable."}`))
		}},
		{name: "Google token endpoint rate limited", start: func(world *calendarDestinationWorld) {
			world.google.answerRenewals(googleTokenAnswer(http.StatusTooManyRequests, `{"error":"rate_limit_exceeded","error_description":"Rate limit exceeded."}`))
		}},
		{name: "network failure reaching Google", start: func(world *calendarDestinationWorld) {
			world.google.answerRenewals(dropGoogleConnection)
		}},
		{name: "renewed token not saved", start: func(world *calendarDestinationWorld) {
			world.store.failSaves(errors.New("connection table unavailable"))
		}},
	} {
		for _, action := range googleWriteActions {
			t.Run(action.name+" with "+failure.name, func(t *testing.T) {
				world := newCalendarDestinationWorld(t)
				world.connectWithDueToken(t)
				kept := world.store.snapshot()
				renewals, calls := world.google.renewalCount(), world.google.callCount()

				failure.start(world)
				answer := action.run(t, world)
				if world.google.renewalCount() == renewals {
					t.Fatalf("%s did not renew the connection's due access token", action.name)
				}
				if !strings.Contains(answer, "try again later") || strings.Contains(answer, "Connect again") {
					t.Fatalf("%s answered %q; want a retry that keeps the connection", action.name, answer)
				}
				if sent := world.google.callsSince(calls); len(sent) != 0 {
					t.Errorf("%s sent event requests %v without a usable token", action.name, sent)
				}
				if !reflect.DeepEqual(world.store.snapshot(), kept) {
					t.Errorf("%s changed the stored connection: %+v, want %+v", action.name, world.store.snapshot(), kept)
				}
				if !world.holdsConnectionCookie() {
					t.Errorf("%s cleared the Google connection cookie", action.name)
				}
				if page := world.page(t); strings.Contains(page, "Not connected") || !strings.Contains(page, calendarAccount) {
					t.Errorf("after the failed %s the page no longer shows the kept connection", action.name)
				}

				// Once Google and the store recover, the same connection writes
				// to the calendar chosen before.
				world.google.answerRenewals(nil)
				world.store.failSaves(nil)
				calls = world.google.callCount()
				retried := action.run(t, world)
				if !strings.Contains(retried, action.done) {
					t.Fatalf("%s after recovery answered %q, want %q", action.name, retried, action.done)
				}
				sent := world.google.callsSince(calls)
				if len(sent) == 0 {
					t.Fatalf("%s after recovery sent no event requests", action.name)
				}
				for _, call := range sent {
					if !strings.Contains(call, " "+teamCalendarID) {
						t.Errorf("%s after recovery sent %q outside the chosen calendar", action.name, call)
					}
				}
			})
		}
	}
}

func TestGoogleRejectingTheGrantAtRenewalRemovesTheConnectionAndAsksToReconnect(t *testing.T) {
	for _, action := range googleWriteActions {
		t.Run(action.name, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connectWithDueToken(t)
			world.google.answerRenewals(googleTokenAnswer(http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
			calls := world.google.callCount()

			answer := action.run(t, world)
			if !strings.Contains(answer, "Connect again") {
				t.Fatalf("%s with a rejected grant answered %q; want a request to reconnect", action.name, answer)
			}
			if sent := world.google.callsSince(calls); len(sent) != 0 {
				t.Errorf("%s with a rejected grant sent event requests %v", action.name, sent)
			}
			if len(world.store.snapshot()) != 0 || world.holdsConnectionCookie() {
				t.Error("a connection whose grant Google rejected was kept")
			}
			if page := world.page(t); !strings.Contains(page, "Not connected") {
				t.Error("the Soccer page still presented the rejected connection")
			}
		})
	}
}

// makeStoredAccessUnreadable replaces every stored connection's token with
// ciphertext the site cannot read, as a changed session key leaves it.
func (world *calendarDestinationWorld) makeStoredAccessUnreadable() {
	world.store.edit(func(records map[string]internalgoogle.ConnectionRecord) {
		for id := range records {
			record := records[id]
			record.TokenCiphertext = "not-a-sealed-token"
			records[id] = record
		}
	})
}

func TestAWriteWithStoredAccessTheSiteCannotReadRemovesTheConnectionAndAsksToReconnect(t *testing.T) {
	for _, action := range googleWriteActions {
		t.Run(action.name, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connect(t)
			world.choose(t, teamCalendarID)
			world.fetch(t)
			world.makeStoredAccessUnreadable()
			renewals, calls := world.google.renewalCount(), world.google.callCount()

			answer := action.run(t, world)
			if !strings.Contains(answer, "Connect again") || strings.Contains(answer, "try again later") {
				t.Fatalf("%s with unreadable stored access answered %q; want a request to reconnect", action.name, answer)
			}
			if world.google.renewalCount() != renewals {
				t.Errorf("%s asked Google to renew access it could not read", action.name)
			}
			if sent := world.google.callsSince(calls); len(sent) != 0 {
				t.Errorf("%s with unreadable stored access sent event requests %v", action.name, sent)
			}
			if len(world.store.snapshot()) != 0 || world.holdsConnectionCookie() {
				t.Error("a connection whose stored access cannot be read was kept")
			}
			if page := world.page(t); !strings.Contains(page, "Not connected") {
				t.Error("the Soccer page still presented the unreadable connection")
			}
		})
	}
}

func TestTheGoogleCardRemovesAConnectionWhoseStoredAccessTheSiteCannotRead(t *testing.T) {
	for _, view := range []struct {
		name string
		show func(t *testing.T, world *calendarDestinationWorld) string
	}{
		{name: "Soccer page", show: func(t *testing.T, world *calendarDestinationWorld) string { return world.page(t) }},
		{name: "calendar choice", show: func(t *testing.T, world *calendarDestinationWorld) string {
			return world.choose(t, teamCalendarID)
		}},
	} {
		t.Run(view.name, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connect(t)
			world.choose(t, teamCalendarID)
			world.makeStoredAccessUnreadable()

			card := view.show(t, world)
			if !strings.Contains(card, "Not connected") || strings.Contains(card, "Try again in a moment") {
				t.Errorf("the %s still presents a connection whose stored access cannot be read", view.name)
			}
			if len(world.store.snapshot()) != 0 || world.holdsConnectionCookie() {
				t.Errorf("the %s kept a connection whose stored access cannot be read", view.name)
			}
		})
	}
}
