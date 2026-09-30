package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// reconnect completes Google Calendar consent again from the signed-in
// browser, over the owner's stored connection when there is one, and returns
// the Soccer page the consent returns to.
func (world *calendarDestinationWorld) reconnect(t *testing.T) string {
	t.Helper()
	completeGoogleConsent(t, world.browser)
	return world.page(t)
}

func TestReconnectingTheSameGoogleAccountKeepsTheChosenCalendar(t *testing.T) {
	world := newCalendarDestinationWorld(t)
	world.connect(t)
	world.choose(t, teamCalendarID)

	page := world.reconnect(t)
	if !strings.Contains(page, calendarReady) || selectedCalendar(t, page) != teamCalendarID || !strings.Contains(page, "Connected to "+teamCalendarName) {
		t.Fatal("reconnecting the same Google account did not keep the chosen calendar ready")
	}
	world.fetch(t)
	if added := world.add(t, nextGameID); !strings.Contains(added, "Added 1 selected game") {
		t.Fatalf("Add after reconnecting answered %q", added)
	}
	if _, ok := world.google.events(teamCalendarID)[nextGameID]; !ok || len(world.google.events(primaryCalendarID)) != 0 {
		t.Fatal("the first Add after reconnecting did not go to the chosen calendar")
	}
}

func TestReconnectingStartsAtPrimaryWhenTheChosenCalendarCannotBeKept(t *testing.T) {
	for _, tc := range []struct {
		name string
		// before changes the world after the owner chose the team calendar
		// and before the browser consents again.
		before func(t *testing.T, world *calendarDestinationWorld)
	}{
		{name: "another Google account consents", before: func(_ *testing.T, world *calendarDestinationWorld) {
			// The other account can write the team calendar too.
			world.google.consentAs("google-coach", "coach@example.net")
		}},
		{name: "another site owner connects the same Google account", before: func(t *testing.T, world *calendarDestinationWorld) {
			if signOut := world.browser.do(httptest.NewRequest(http.MethodPost, "https://app.example.com/sign-out", nil)); signOut.Code != http.StatusSeeOther {
				t.Fatalf("site sign-out status = %d", signOut.Code)
			}
			signInAsInvitedOwner(t, world.app, world.cognito, world.browser, journeySecondOwnerEmail, journeySecondOwnerSubject)
		}},
		{name: "chosen calendar removed from the account", before: func(_ *testing.T, world *calendarDestinationWorld) {
			world.google.setAccess(teamCalendarID, "")
		}},
		{name: "chosen calendar now read-only", before: func(_ *testing.T, world *calendarDestinationWorld) {
			world.google.setAccess(teamCalendarID, "reader")
		}},
		{name: "writes paused after the chosen calendar was lost", before: func(t *testing.T, world *calendarDestinationWorld) {
			world.google.setAccess(teamCalendarID, "")
			if page := world.page(t); !strings.Contains(page, calendarChoiceNeeded) {
				t.Fatal("the page did not pause writes to the lost calendar")
			}
			// The calendar returning does not resume the paused choice.
			world.google.setAccess(teamCalendarID, "writer")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			world := newCalendarDestinationWorld(t)
			world.connect(t)
			world.choose(t, teamCalendarID)
			tc.before(t, world)

			page := world.reconnect(t)
			if !strings.Contains(page, calendarReady) || selectedCalendar(t, page) != primaryCalendarID || !strings.Contains(page, "Connected to "+primaryCalendarName) {
				t.Fatal("the new connection did not start at the primary calendar, ready")
			}
			world.fetch(t)
			if added := world.add(t, nextGameID); !strings.Contains(added, "Added 1 selected game") {
				t.Fatalf("Add after reconnecting answered %q", added)
			}
			if _, ok := world.google.events(primaryCalendarID)[nextGameID]; !ok || len(world.google.events(teamCalendarID)) != 0 {
				t.Fatal("the first Add after reconnecting did not go to the primary calendar")
			}
		})
	}
}
