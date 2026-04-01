package soccer

import (
	"net/url"
	"reflect"
	"testing"
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
