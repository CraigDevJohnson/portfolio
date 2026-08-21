package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"golang.org/x/net/html"

	"portfolio/cmd/web/pages"
	"portfolio/cmd/web/partials"
	"portfolio/types"
)

func TestSoccerPageRendersMatchdayPlannerContract(t *testing.T) {
	html := renderSoccerTestComponent(t, pages.Soccer(soccerPresentationTestPageProps()))

	if !strings.Contains(html, `data-layout="matchday-planner"`) {
		t.Fatalf("Soccer page lacks matchday planner layout marker")
	}
	if got := strings.Count(html, `<h1`); got != 1 {
		t.Fatalf("Soccer page h1 count = %d, want 1", got)
	}
	if got := strings.Count(html, `data-signal-trail`); got != 1 {
		t.Fatalf("Soccer page signal trail count = %d, want 1", got)
	}
	if !strings.Contains(html, `signal-trail-matchday`) {
		t.Fatalf("Soccer page lacks typed matchday trail")
	}

	connections := strings.Index(html, `id="soccer-connections"`)
	if connections < 0 {
		t.Fatal("Soccer page lacks the Connections panel")
	}
	wantStages := []string{"source", "players", "teams", "review"}
	last := -1
	for _, stage := range wantStages {
		marker := `data-soccer-stage="` + stage + `"`
		if got := strings.Count(html, marker); got != 1 {
			t.Fatalf("Soccer stage %q count = %d, want 1", stage, got)
		}
		index := strings.Index(html, marker)
		if index <= last {
			t.Fatalf("Soccer stage %q is not after the previous stage", stage)
		}
		last = index
	}
	if connections >= strings.Index(html, `data-soccer-stage="source"`) {
		t.Fatal("Soccer Connections panel must precede stage 1")
	}
	if strings.Contains(html, `data-soccer-stage="calendar-output"`) || strings.Contains(html, `<strong>Calendar output</strong>`) {
		t.Fatal("Soccer page retains a fifth Calendar Output stage")
	}
	workspaceEnd := strings.Index(html, `</div></div><section class="soccer-stage soccer-stage-review`)
	if workspaceEnd < 0 {
		t.Fatal("Review & Output is not rendered as a full-width section after the planner workspace")
	}
}

func TestSoccerPrimaryPlayerOwnsWholeRowAndSubClassificationIsRemoved(t *testing.T) {
	players := renderSoccerTestComponent(t, partials.SoccerPlayerSelect(partials.SoccerPlayerSelectProps{Players: []types.LPSPlayer{
		{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
		{UPlayerID: 1002, FirstName: "Taylor", LastName: "Johnson"},
	}}))
	if got := strings.Count(players, `data-primary-player="true"`); got != 1 {
		t.Fatalf("primary player row marker count = %d, want 1", got)
	}
	if !strings.Contains(players, "Primary player") || strings.Contains(players, `>Primary</span>`) {
		t.Errorf("primary player supporting copy is not integrated into the row: %s", players)
	}

	teams := renderSoccerTestComponent(t, partials.SoccerTeamSelect(partials.SoccerTeamSelectProps{
		PlayerGroups: []types.PlayerTeamGroup{{
			Player: types.LPSPlayer{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson"},
			Teams:  []types.LPSTeam{{TeamID: 4001, TeamName: "One", PlayerID: 1001}, {TeamID: 4002, TeamName: "Two", PlayerID: 1001}},
		}},
		PlayerIDs: []int{1001},
	}))
	if strings.Contains(teams, "Sub") {
		t.Errorf("team selection retains sub-team classification: %s", teams)
	}
	if inputs, checked := strings.Count(teams, `name="team_ids" value=`), strings.Count(teams, " checked"); inputs != 2 || checked != 2 {
		t.Errorf("team selection inputs/checked = %d/%d, want 2/2: %s", inputs, checked, teams)
	}

	encoded, err := json.Marshal(types.LPSTeam{TeamID: 4001})
	if err != nil {
		t.Fatalf("marshal LPSTeam: %v", err)
	}
	if strings.Contains(string(encoded), "is_sub_team") {
		t.Fatalf("LPSTeam JSON retains is_sub_team: %s", encoded)
	}
}

func TestSoccerConnectionCardsExposeExplicitState(t *testing.T) {
	connected := renderSoccerTestComponent(t, partials.SoccerConnections(partials.SoccerLoginStateProps{
		Authenticated:         true,
		LoginAvailable:        true,
		GoogleAvailable:       true,
		GoogleConnected:       true,
		GoogleCalendarSummary: "Matchdays",
		SelectedPlayerIDs:     []int{1001},
		SelectedTeamIDs:       []int{4101},
		Players: []types.LPSPlayer{
			{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
			{UPlayerID: 1002, FirstName: "Taylor", LastName: "Johnson"},
		},
	}))
	for _, marker := range []string{
		`id="soccer-lps-connection"`, `id="soccer-google-connection"`,
		`data-connection-state="connected"`, "Imported for this session", "Connected to Matchdays",
		"2 linked players", "1 selected player", "1 confirmed team", "Calendar ready", "Matchdays",
	} {
		if !strings.Contains(connected, marker) {
			t.Errorf("connected Soccer cards lack %q", marker)
		}
	}
	if strings.Contains(connected, `id="soccer-configuration"`) || strings.Contains(connected, "View configuration") {
		t.Error("connected Soccer cards still duplicate their state in a separate configuration dashboard")
	}

	disconnected := renderSoccerTestComponent(t, partials.SoccerConnections(partials.SoccerLoginStateProps{LoginAvailable: true, GoogleAvailable: true}))
	if got := strings.Count(disconnected, `data-connection-state="disconnected"`); got != 2 {
		t.Errorf("disconnected connection-state count = %d, want 2", got)
	}
	for _, marker := range []string{"Not imported", "Not connected"} {
		if !strings.Contains(disconnected, marker) {
			t.Errorf("disconnected Soccer cards lack %q", marker)
		}
	}

	unavailable := renderSoccerTestComponent(t, partials.SoccerConnections(partials.SoccerLoginStateProps{}))
	if got := strings.Count(unavailable, `data-connection-state="unavailable"`); got != 2 {
		t.Errorf("unavailable connection-state count = %d, want 2", got)
	}
	for _, marker := range []string{"Not enabled on this server", "Use manual team IDs"} {
		if !strings.Contains(unavailable, marker) {
			t.Errorf("unavailable Soccer cards lack %q", marker)
		}
	}
	if strings.Contains(unavailable, "Manual lookup is ready") {
		t.Error("unavailable Soccer cards still describe manual lookup as configured access")
	}
}

func TestSoccerSourceStageOffersTheNextActionWithoutRepeatingImport(t *testing.T) {
	tests := []struct {
		name      string
		state     partials.SoccerLoginStateProps
		wantState string
		wantCopy  string
		wantHook  string
	}{
		{
			name:      "imported",
			state:     partials.SoccerLoginStateProps{Authenticated: true, LoginAvailable: true, Players: []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig"}}},
			wantState: "ready",
			wantCopy:  "Use linked players",
			wantHook:  `href="#soccer-player-stage"`,
		},
		{
			name:      "available",
			state:     partials.SoccerLoginStateProps{LoginAvailable: true},
			wantState: "locked",
			wantCopy:  "Set up LPS access in Connections above",
		},
		{
			name:      "unavailable",
			state:     partials.SoccerLoginStateProps{},
			wantState: "locked",
			wantCopy:  "Use manual team IDs",
			wantHook:  `href="#team_codes"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			html := renderSoccerTestComponent(t, pages.Soccer(pages.SoccerProps{AuthState: test.state}))
			for _, marker := range []string{`data-source-state="` + test.wantState + `"`, test.wantCopy, test.wantHook} {
				if marker == "" {
					continue
				}
				if !strings.Contains(html, marker) {
					t.Errorf("Soccer %s source stage lacks %q", test.name, marker)
				}
			}
			sourceStart := strings.Index(html, `data-soccer-stage="source"`)
			sourceEnd := strings.Index(html, `data-soccer-stage="players"`)
			if sourceStart < 0 || sourceEnd <= sourceStart {
				t.Fatalf("Soccer %s source-stage boundaries are missing", test.name)
			}
			source := html[sourceStart:sourceEnd]
			for _, duplicate := range []string{"Import access", "Imported context", "JWT", `data-open-login-modal`} {
				if strings.Contains(source, duplicate) {
					t.Errorf("Soccer %s source stage repeats the Connections import UI with %q", test.name, duplicate)
				}
			}
		})
	}
}

func TestSoccerPlannerExplainsTheFourStageWorkflow(t *testing.T) {
	html := renderSoccerTestComponent(t, pages.Soccer(soccerPresentationTestPageProps()))
	legendStart := strings.Index(html, `class="soccer-stage-legend"`)
	if legendStart < 0 {
		t.Fatal("Soccer page lacks the workflow legend")
	}
	legendEnd := strings.Index(html[legendStart:], `</ol>`)
	if legendEnd < 0 {
		t.Fatal("Soccer workflow legend is incomplete")
	}
	legend := html[legendStart : legendStart+legendEnd]
	if got := strings.Count(legend, `<li>`); got != 4 {
		t.Fatalf("Soccer workflow legend step count = %d, want 4", got)
	}
	for _, marker := range []string{
		"Choose source", "Select players", "Confirm teams", "Review &amp; output",
	} {
		if !strings.Contains(legend, marker) {
			t.Errorf("Soccer workflow legend lacks %q", marker)
		}
	}
	for _, duplicate := range []string{"Import access", "Connect calendar", "Add or sync"} {
		if strings.Contains(legend, duplicate) {
			t.Errorf("Soccer workflow legend still duplicates connection/output action %q", duplicate)
		}
	}
}

func TestSoccerFragmentsRenderWithoutSignalTrail(t *testing.T) {
	props := soccerPresentationTestPageProps()
	fragments := map[string]templ.Component{
		"auth":    partials.SoccerLoginState(props.AuthState),
		"modal":   partials.SoccerLoginModal(partials.SoccerLoginModalProps{}),
		"players": partials.SoccerPlayerSelect(partials.SoccerPlayerSelectProps{Players: props.AuthState.Players}),
		"teams": partials.SoccerTeamSelect(partials.SoccerTeamSelectProps{
			PlayerGroups: []types.PlayerTeamGroup{{
				Player: props.AuthState.Players[0],
				Teams:  []types.LPSTeam{{TeamID: 4001, TeamName: "Pond Mint United", Season: 84, PlayerID: 1001}},
			}},
			PlayerIDs: []int{1001},
		}),
		"table": partials.SoccerTableFragment(soccerPresentationTestTableProps()),
		"feedback": partials.SoccerLoginFeedback(partials.SoccerLoginFeedbackProps{
			Kind:    "error",
			Message: "The schedule could not be loaded.",
		}),
	}

	for name, component := range fragments {
		t.Run(name, func(t *testing.T) {
			html := renderSoccerTestComponent(t, component)
			if got := strings.Count(html, `data-signal-trail`); got != 0 {
				t.Fatalf("%s fragment signal trail count = %d, want 0", name, got)
			}
			if strings.Contains(html, `data-layout="matchday-planner"`) {
				t.Fatalf("%s fragment unexpectedly renders full-page layout", name)
			}
		})
	}
}

func TestSoccerProductionFormsPreserveEndpointsFieldsAndHooks(t *testing.T) {
	pageHTML := renderSoccerTestComponent(t, pages.Soccer(soccerPresentationTestPageProps()))
	importHTML := renderSoccerTestComponent(t, partials.SoccerLoginState(partials.SoccerLoginStateProps{LoginAvailable: true}))
	tableHTML := renderSoccerTestComponent(t, partials.SoccerTableFragment(soccerPresentationTestTableProps()))
	teamHTML := renderSoccerTestComponent(t, partials.SoccerTeamSelect(partials.SoccerTeamSelectProps{
		PlayerGroups: []types.PlayerTeamGroup{{
			Player: types.LPSPlayer{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
			Teams:  []types.LPSTeam{{TeamID: 4001, TeamName: "Pond Mint United", Season: 84, PlayerID: 1001}},
		}},
		PlayerIDs: []int{1001},
	}))
	combined := pageHTML + importHTML + tableHTML + teamHTML

	for _, marker := range []string{
		`id="soccer-lps-connection"`, `id="soccer-google-connection"`, `id="soccer-login-modal"`, `id="soccer-login-title"`,
		`id="soccer-login-description"`, `id="soccer-login-form"`, `id="soccer-import-jwt"`,
		`id="soccer-import-jwt-hint"`, `id="soccer-login-feedback"`, `id="fetch-form"`,
		`id="team_codes"`, `id="team-codes-hint"`, `id="soccer-player-select-form"`,
		`id="soccer-team-select-form"`, `id="soccer-google-calendar"`, `id="games-container"`,
		`id="loading-indicator"`, `id="upcoming-games-form"`, `id="past-results-form"`,
		`id="download-button"`, `id="upcoming-calendar-feedback"`, `id="past-results-feedback"`, `id="upcoming-games-heading"`,
		`id="past-results-heading"`,
		`hx-post="/soccer/import"`, `hx-post="/soccer/logout"`,
		`hx-post="/soccer/discover-teams"`, `hx-post="/soccer/fetch"`,
		`action="/soccer/download"`, `hx-post="/soccer/google/disconnect"`,
		`hx-post="/soccer/google/calendar"`, `hx-post="/soccer/google/add"`,
		`hx-post="/soccer/google/sync-results"`, `href="/soccer/google/connect"`,
		`name="jwt"`, `name="team_codes"`, `name="player_ids"`, `name="team_ids"`,
		`name="selection_mode" value="teams"`, `name="selected"`, `name="calendar_id"`,
		`data-open-login-modal`, `data-close-login-modal`, `data-loading-button`,
		`data-loading-link`, `data-soccer-form`, `data-native-download`, `data-game-checkbox`,
		`data-game-group`, `data-game-action`, `data-select-all`, `data-selected-count`, `data-past-game`,
	} {
		if !strings.Contains(combined, marker) {
			t.Errorf("Soccer production rendering lacks contract marker %q", marker)
		}
	}
	formStart := strings.Index(tableHTML, `<form id="upcoming-games-form"`)
	if formStart < 0 {
		t.Fatal("native ICS form is missing")
	}
	formEnd := strings.Index(tableHTML[formStart:], ">")
	if formEnd < 0 {
		t.Fatal("native ICS form opening tag is incomplete")
	}
	formTag := tableHTML[formStart : formStart+formEnd+1]
	for _, marker := range []string{`action="/soccer/download"`, `method="post"`, `data-native-download`} {
		if !strings.Contains(formTag, marker) {
			t.Errorf("native ICS form opening tag lacks %q: %s", marker, formTag)
		}
	}
}

func TestSoccerLoginFeedbackPrecedesEntryControls(t *testing.T) {
	html := renderSoccerTestComponent(t, partials.SoccerLoginModal(partials.SoccerLoginModalProps{
		Open: true,
		Feedback: &partials.SoccerLoginFeedbackProps{
			Kind:    "error",
			Title:   "Token expired",
			Message: "Copy a fresh token and try again.",
		},
	}))

	formStart := strings.Index(html, `id="soccer-login-form"`)
	feedbackStart := strings.Index(html, `id="soccer-login-feedback"`)
	entryStart := strings.Index(html, `id="soccer-import-jwt"`)
	actionsStart := strings.Index(html, `class="soccer-login-actions`)
	if formStart < 0 || feedbackStart < 0 || entryStart < 0 || actionsStart < 0 {
		t.Fatalf("Soccer modal lacks required form anatomy: form=%d feedback=%d entry=%d actions=%d", formStart, feedbackStart, entryStart, actionsStart)
	}
	if !(formStart < feedbackStart && feedbackStart < entryStart && entryStart < actionsStart) {
		t.Fatalf("Soccer modal must render feedback before entry controls and sticky actions: form=%d feedback=%d entry=%d actions=%d", formStart, feedbackStart, entryStart, actionsStart)
	}
}

func TestSoccerLoginJWTFieldUsesFocusRingClearanceStack(t *testing.T) {
	html := renderSoccerTestComponent(t, partials.SoccerLoginModal(partials.SoccerLoginModalProps{Open: true}))

	if err := validateSoccerLoginJWTFieldMarkup(html); err != nil {
		t.Fatal(err)
	}
}

func TestSoccerLoginJWTFieldMarkupValidatorRejectsOwnershipRegressions(t *testing.T) {
	const valid = `<form><div class="soccer-login-jwt-field"><label for="soccer-import-jwt">JWT</label><textarea id="soccer-import-jwt"></textarea><p id="soccer-import-jwt-hint">Hint</p></div></form>`
	mutations := map[string]string{
		"class moved to unrelated div": strings.Replace(valid, `<div class="soccer-login-jwt-field"><label`, `<div class="soccer-login-jwt-field"></div><div><label`, 1),
		"hint moved outside wrapper":   strings.Replace(valid, `<p id="soccer-import-jwt-hint">Hint</p></div>`, `</div><p id="soccer-import-jwt-hint">Hint</p>`, 1),
		"hint precedes textarea":       strings.Replace(valid, `<textarea id="soccer-import-jwt"></textarea><p id="soccer-import-jwt-hint">Hint</p>`, `<p id="soccer-import-jwt-hint">Hint</p><textarea id="soccer-import-jwt"></textarea>`, 1),
	}
	if err := validateSoccerLoginJWTFieldMarkup(valid); err != nil {
		t.Fatalf("valid JWT field markup rejected: %v", err)
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			if err := validateSoccerLoginJWTFieldMarkup(mutation); err == nil {
				t.Fatal("JWT field markup regression was accepted")
			}
		})
	}
}

func validateSoccerLoginJWTFieldMarkup(markup string) error {
	document, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		return fmt.Errorf("parse Soccer modal markup: %w", err)
	}
	var owners []*html.Node
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && soccerHTMLClassContains(node, "soccer-login-jwt-field") {
			owners = append(owners, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	if len(owners) != 1 {
		return fmt.Errorf("Soccer modal JWT focus-clearance stack count = %d, want 1", len(owners))
	}
	var children []*html.Node
	for child := owners[0].FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode {
			children = append(children, child)
		}
	}
	if len(children) != 3 {
		return fmt.Errorf("Soccer modal JWT focus-clearance stack direct element count = %d, want label, textarea, hint", len(children))
	}
	if children[0].Data != "label" || soccerHTMLAttribute(children[0], "for") != "soccer-import-jwt" {
		return fmt.Errorf("Soccer modal JWT focus-clearance stack first child is not its exact label")
	}
	if children[1].Data != "textarea" || soccerHTMLAttribute(children[1], "id") != "soccer-import-jwt" {
		return fmt.Errorf("Soccer modal JWT focus-clearance stack second child is not the JWT field")
	}
	if children[2].Data != "p" || soccerHTMLAttribute(children[2], "id") != "soccer-import-jwt-hint" {
		return fmt.Errorf("Soccer modal JWT focus-clearance stack third child is not the described hint")
	}
	return nil
}

func soccerHTMLClassContains(node *html.Node, className string) bool {
	for _, class := range strings.Fields(soccerHTMLAttribute(node, "class")) {
		if class == className {
			return true
		}
	}
	return false
}

func soccerHTMLAttribute(node *html.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func TestSoccerStageTransitionsAndRecoveryActionsStayReachable(t *testing.T) {
	players := renderSoccerTestComponent(t, partials.SoccerPlayerSelect(partials.SoccerPlayerSelectProps{
		Players: []types.LPSPlayer{{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson"}},
	}))
	for _, marker := range []string{`hx-post="/soccer/discover-teams"`, `hx-target="#soccer-team-stage-content"`} {
		if !strings.Contains(players, marker) {
			t.Errorf("player selection lacks stage-3 transition marker %q", marker)
		}
	}
	if strings.Contains(players, `hx-indicator="#loading-indicator`) {
		t.Error("player discovery activates the Stage 4 schedule-loading indicator while targeting Stage 3")
	}
	if !strings.Contains(players, `hx-indicator=".soccer-player-submit-indicator"`) {
		t.Error("player discovery lacks its initiating-control indicator")
	}

	noPlayers := renderSoccerTestComponent(t, partials.SoccerPlayerStageContent(partials.SoccerLoginStateProps{Authenticated: true, LoginAvailable: true}))
	for _, marker := range []string{"No linked players were found for this account.", `data-open-login-modal`, "Replace access"} {
		if !strings.Contains(noPlayers, marker) {
			t.Errorf("no-player recovery lacks %q", marker)
		}
	}
	if strings.Contains(noPlayers, "token") {
		t.Error("no-player recovery reintroduces a second token-import path")
	}

	noGames := renderSoccerTestComponent(t, partials.SoccerTableFragment(partials.SoccerTableFragmentProps{Message: "No games found for the provided request.", ImportAvailable: true}))
	for _, marker := range []string{`href="#team_codes"`, `data-open-login-modal`, "Import fresh access"} {
		if !strings.Contains(noGames, marker) {
			t.Errorf("no-games recovery lacks %q", marker)
		}
	}
	manualOnly := renderSoccerTestComponent(t, partials.SoccerTableFragment(partials.SoccerTableFragmentProps{Message: "No games found for the provided request."}))
	if strings.Contains(manualOnly, `data-open-login-modal`) || strings.Contains(manualOnly, "Import fresh access") {
		t.Error("manual-only no-games state advertises an unavailable import action")
	}
}

func TestSoccerLoadingIndicatorIsAssociatedAndAnnounceable(t *testing.T) {
	html := renderSoccerTestComponent(t, pages.Soccer(soccerPresentationTestPageProps()))
	for _, marker := range []string{
		`id="loading-indicator"`, `role="status"`, `aria-live="polite"`,
		`aria-atomic="true"`, `aria-describedby="loading-indicator"`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("Soccer loading anatomy lacks %q", marker)
		}
	}
	if strings.Contains(html, `id="loading-indicator" class="loading-indicator soccer-results-loading htmx-indicator" aria-hidden="true"`) {
		t.Error("production schedule loading status remains permanently hidden from assistive technology")
	}
}

func TestSoccerLoginStateSeparatesGoogleRefreshFromWorkflowReset(t *testing.T) {
	state := soccerPresentationTestPageProps().AuthState
	state.RefreshCalendar = true
	googleRefresh := renderSoccerTestComponent(t, partials.SoccerLoginState(state))
	for _, marker := range []string{
		`id="soccer-lps-connection"`,
		`id="soccer-google-connection"`,
	} {
		if !strings.Contains(googleRefresh, marker) {
			t.Errorf("Google-only refresh lacks %q", marker)
		}
	}
	lpsStart := strings.Index(googleRefresh, `id="soccer-lps-connection"`)
	if lpsStart < 0 {
		t.Fatal("Google-only refresh lacks LPS connection opening tag")
	}
	lpsEnd := strings.Index(googleRefresh[lpsStart:], `>`)
	if lpsEnd < 0 {
		t.Fatal("Google-only refresh LPS connection opening tag is incomplete")
	}
	if strings.Contains(googleRefresh[lpsStart:lpsStart+lpsEnd+1], `hx-swap-oob`) {
		t.Error("Google-only refresh marks its primary LPS target OOB")
	}
	googleStart := strings.Index(googleRefresh, `id="soccer-google-connection"`)
	if googleStart < 0 {
		t.Fatal("Google-only refresh lacks Google connection opening tag")
	}
	googleEnd := strings.Index(googleRefresh[googleStart:], `>`)
	if googleEnd < 0 || !strings.Contains(googleRefresh[googleStart:googleStart+googleEnd+1], `hx-swap-oob="outerHTML"`) {
		t.Error("Google-only refresh does not replace the Google connection OOB")
	}
	if strings.Contains(googleRefresh, `id="soccer-configuration"`) {
		t.Error("Google-only refresh revives the removed duplicate configuration summary")
	}
	for _, staleSafeID := range []string{"soccer-player-stage-content", "soccer-team-stage-content", "games-container"} {
		if strings.Contains(googleRefresh, `id="`+staleSafeID+`"`) {
			t.Errorf("Google-only refresh unexpectedly replaces %s", staleSafeID)
		}
	}

	state.ResetWorkflow = true
	workflowReset := renderSoccerTestComponent(t, partials.SoccerLoginState(state))
	for _, resetID := range []string{
		"soccer-player-stage-content", "soccer-team-stage-content", "games-container",
	} {
		if !strings.Contains(workflowReset, `id="`+resetID+`" hx-swap-oob="innerHTML"`) {
			t.Errorf("workflow reset lacks OOB replacement for %s", resetID)
		}
	}
	if strings.Contains(workflowReset[lpsStart:lpsStart+lpsEnd+1], `hx-swap-oob`) {
		t.Error("workflow reset marks its primary LPS target OOB")
	}

	state.SwapOOB = true
	state.RefreshCalendar = false
	importResponse := renderSoccerTestComponent(t, partials.SoccerLoginState(state))
	importLPSStart := strings.Index(importResponse, `id="soccer-lps-connection"`)
	if importLPSStart < 0 {
		t.Fatal("import response lacks LPS connection opening tag")
	}
	importLPSEnd := strings.Index(importResponse[importLPSStart:], `>`)
	if importLPSEnd < 0 || !strings.Contains(importResponse[importLPSStart:importLPSStart+importLPSEnd+1], `hx-swap-oob="outerHTML"`) {
		t.Error("import response does not replace the LPS connection OOB")
	}
	if strings.Contains(importResponse, `id="soccer-google-connection"`) {
		t.Error("LPS import response unexpectedly replaces the Google connection")
	}
	if strings.Contains(importResponse, `id="soccer-configuration"`) {
		t.Error("LPS import response revives the removed duplicate configuration summary")
	}
}

func TestSoccerDisconnectedPageHasOneVisibleImportLauncher(t *testing.T) {
	props := soccerPresentationTestPageProps()
	props.AuthState.Authenticated = false
	props.AuthState.Players = nil
	props.AuthState.SelectedPlayerIDs = nil

	html := renderSoccerTestComponent(t, pages.Soccer(props))
	if got := strings.Count(html, `data-open-login-modal`); got != 1 {
		t.Fatalf("visible Import access launcher count = %d, want one canonical launcher", got)
	}
	if !strings.Contains(html, `href="#soccer-connections"`) {
		t.Fatal("disconnected source stage does not direct users to the canonical Connections setup")
	}
}

func TestSoccerScheduleUsesResponsiveMatchListsWithLocalFeedback(t *testing.T) {
	html := renderSoccerTestComponent(t, partials.SoccerTableFragment(soccerPresentationTestTableProps()))
	if got := strings.Count(html, `class="games-total-count">1 game</span>`); got != 2 {
		t.Errorf("single-game total labels = %d, want 2 correctly pluralized labels", got)
	}
	for _, section := range []string{"upcoming-games", "past-results"} {
		for _, marker := range []string{
			`class="soccer-match-list"`,
			`aria-labelledby="` + section + `-heading"`,
			`data-game-group="` + section + `"`,
		} {
			if !strings.Contains(html, marker) {
				t.Errorf("%s responsive match-list contract lacks %q", section, marker)
			}
		}
	}
	for _, marker := range []string{
		`id="upcoming-calendar-feedback"`, `id="past-results-feedback"`,
		`role="status"`, `aria-live="polite"`, `data-soccer-feedback`,
		`hx-target="#upcoming-calendar-feedback"`, `hx-target="#past-results-feedback"`,
		`data-team-fingerprint="4001"`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("schedule local feedback contract lacks %q", marker)
		}
	}
	for _, forbidden := range []string{`class="table-wrapper"`, `class="games-table"`, "table-scroll-hint", "Scroll horizontally", `id="google-calendar-feedback"`} {
		if strings.Contains(html, forbidden) {
			t.Errorf("responsive schedule retains forbidden table/overflow marker %q", forbidden)
		}
	}
	if got, want := strings.Count(html, `class="game-checkbox-target"`), 2; got != want {
		t.Errorf("game row checkbox target count = %d, want %d", got, want)
	}
	for _, marker := range []string{"Pond Mint United", "Campfire Rovers", "Venue not provided", "Field 2", "Fall 2026"} {
		if !strings.Contains(html, marker) {
			t.Errorf("match row lacks %q", marker)
		}
	}
}

func TestSoccerPreviewSuccessFixturesPreserveHandlerMessages(t *testing.T) {
	tests := []struct{ fixture, message string }{
		{fixture: "google-add-success", message: "Added 2 selected game(s) to Google Calendar."},
		{fixture: "google-sync-success", message: "2 game result(s) updated in Google Calendar."},
	}
	for _, test := range tests {
		fixture, ok := soccerPreviewFixture(test.fixture)
		if !ok || fixture.Results == nil || fixture.Results.GoogleFeedback == nil {
			t.Fatalf("fixture %q lacks Google feedback", test.fixture)
		}
		if fixture.Results.GoogleFeedback.Message != test.message {
			t.Errorf("fixture %q feedback = %q, want %q", test.fixture, fixture.Results.GoogleFeedback.Message, test.message)
		}
	}
}

func soccerPresentationTestPageProps() pages.SoccerProps {
	return pages.SoccerProps{AuthState: partials.SoccerLoginStateProps{
		Authenticated:            true,
		LoginAvailable:           true,
		GoogleAvailable:          true,
		GoogleConnected:          true,
		GoogleCalendarSummary:    "Matchday Calendar",
		SelectedGoogleCalendarID: "matchday",
		GoogleCalendars: []types.GoogleCalendarOption{{
			ID: "matchday", Summary: "Matchday Calendar", Primary: true,
		}},
		Players: []types.LPSPlayer{{
			UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true,
		}},
	}}
}

func soccerPresentationTestTableProps() partials.SoccerTableFragmentProps {
	return partials.SoccerTableFragmentProps{
		UpcomingGames: []types.Game{{
			ID: "upcoming-1", DateTime: "Aug 20, 2026 at 7:15 PM", Field: "Field 2", Home: "Pond Mint United", Away: "Campfire Rovers", Season: "Fall 2026",
		}},
		PastGames: []types.Game{{
			ID: "past-1", DateTime: "Aug 6, 2026 at 8:20 PM", Field: "Field 1", Home: "Pond Mint United", Away: "Rosehip Athletic", Season: "Summer 2026", PlayerTeamName: "Pond Mint United", Result: "4 - 2",
		}},
		TeamCodes:       "4001",
		PlayerIDs:       []int{1001},
		GoogleAvailable: true,
		GoogleConnected: true,
	}
}

func renderSoccerTestComponent(t *testing.T, component templ.Component) string {
	t.Helper()
	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatalf("render component: %v", err)
	}
	return output.String()
}
