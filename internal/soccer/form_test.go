package soccer

import (
	"net/url"
	"reflect"
	"testing"

	"portfolio/types"
)

func TestParseSelectedIDsTrimsAndDeduplicates(t *testing.T) {
	form := url.Values{
		"selected": {" game-1 ", "", "game-2", "game-1"},
	}

	selected := parseSelectedIDs(form)

	if len(selected) != 2 {
		t.Fatalf("unexpected selected count: got %d want 2", len(selected))
	}
	if _, ok := selected["game-1"]; !ok {
		t.Fatal("expected trimmed game-1 to be selected")
	}
	if _, ok := selected["game-2"]; !ok {
		t.Fatal("expected game-2 to be selected")
	}
}

func TestParsePlayerIDsSkipsInvalidValuesAndDeduplicates(t *testing.T) {
	playerIDs := parsePlayerIDs([]string{" 1001 ", "0", "-7", "abc", "1002", "1001"})

	if !reflect.DeepEqual(playerIDs, []int{1001, 1002}) {
		t.Fatalf("unexpected player IDs: got %#v want %#v", playerIDs, []int{1001, 1002})
	}
}

func TestParseTeamIDsSupportsMixedDelimiters(t *testing.T) {
	teamIDs := parseTeamIDs(" 479691, 479147; 479691  479999 ")

	if !reflect.DeepEqual(teamIDs, []int{479691, 479147, 479999}) {
		t.Fatalf("unexpected team IDs: got %#v want %#v", teamIDs, []int{479691, 479147, 479999})
	}
}

func TestParseScheduleFormInputTracksExplicitTeamSelection(t *testing.T) {
	input := parseScheduleFormInput(url.Values{
		"selection_mode": {teamSelectionMode},
		"player_ids":     {"1001"},
	})

	if !input.TeamSelection {
		t.Fatal("expected explicit team selection mode")
	}
	if !reflect.DeepEqual(input.PlayerIDs, []int{1001}) {
		t.Fatalf("unexpected player IDs: got %#v want %#v", input.PlayerIDs, []int{1001})
	}
	if len(input.TeamIDs) != 0 {
		t.Fatalf("unexpected team IDs: got %#v want none", input.TeamIDs)
	}
}

func TestHasInvalidPlayerInput(t *testing.T) {
	tests := []struct {
		name      string
		rawValues []string
		playerIDs []int
		want      bool
	}{
		{
			name:      "false when parsed players exist",
			rawValues: []string{"1001", "1002"},
			playerIDs: []int{1001, 1002},
			want:      false,
		},
		{
			name:      "false when raw values are blank",
			rawValues: []string{" ", ""},
			playerIDs: nil,
			want:      false,
		},
		{
			name:      "true when raw values remain but parse failed",
			rawValues: []string{"abc", " "},
			playerIDs: nil,
			want:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasInvalidPlayerInput(tc.rawValues, tc.playerIDs); got != tc.want {
				t.Fatalf("unexpected invalid-player result: got %t want %t", got, tc.want)
			}
		})
	}
}

func TestNormalizeWorkflowStateValidatesSourcePlayersAndCompactTeamIDs(t *testing.T) {
	players := []types.LPSPlayer{{UPlayerID: 1001}, {UPlayerID: 1002}}
	state := normalizeWorkflowState(&types.SoccerWorkflowState{
		Source:            "imported",
		SelectedPlayerIDs: []int{1002, 0, 1001, 1002, 9999},
		SelectedTeamIDs:   []int{2001, 0, 9999, 2002, 2001},
	}, players)

	if !reflect.DeepEqual(state.SelectedPlayerIDs, []int{1002, 1001}) {
		t.Fatalf("normalized player IDs = %#v, want [1002 1001]", state.SelectedPlayerIDs)
	}
	if !reflect.DeepEqual(state.SelectedTeamIDs, []int{2001, 9999, 2002}) {
		t.Fatalf("normalized selected team IDs = %#v, want [2001 9999 2002]", state.SelectedTeamIDs)
	}
}

func TestNormalizeWorkflowStateKeepsManualTeamsAndRejectsUnknownSource(t *testing.T) {
	manual := normalizeWorkflowState(&types.SoccerWorkflowState{
		Source:            "manual",
		SelectedPlayerIDs: []int{1001},
		SelectedTeamIDs:   []int{3002, 0, 3001, 3002},
	}, []types.LPSPlayer{{UPlayerID: 1001}})
	if manual.Source != "manual" || len(manual.SelectedPlayerIDs) != 0 || !reflect.DeepEqual(manual.SelectedTeamIDs, []int{3002, 3001}) {
		t.Fatalf("normalized manual workflow = %#v", manual)
	}

	unknown := normalizeWorkflowState(&types.SoccerWorkflowState{Source: "other", SelectedTeamIDs: []int{1}}, nil)
	if !reflect.DeepEqual(unknown, types.SoccerWorkflowState{}) {
		t.Fatalf("unknown workflow source normalized to %#v, want zero value", unknown)
	}
}
