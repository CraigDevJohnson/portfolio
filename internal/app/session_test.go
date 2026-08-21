package app

import (
	"reflect"
	"testing"
	"time"

	"portfolio/types"
)

func TestEncryptDecryptSessionRoundTrip(t *testing.T) {
	app := newTestApp(t)

	expected := types.SessionData{
		JWT:      "token-value",
		UserName: "Craig Johnson",
		Players: []types.LPSPlayer{
			{UPlayerID: 1001, FirstName: "Craig", LastName: "Johnson", IsMainPlayer: true},
		},
		ExpiresAt: time.Unix(1_773_634_161, 0).UTC(),
		Workflow: types.SoccerWorkflowState{
			Source:            "imported",
			SelectedPlayerIDs: []int{1001},
			SelectedTeamIDs:   []int{4101},
		},
	}

	encrypted := encryptTestSession(t, app, &expected)
	actual := decryptTestSession(t, app, encrypted)

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("decryptSession mismatch: got %#v want %#v", actual, expected)
	}
}

func TestDecryptSessionWithoutWorkflowRemainsBackwardCompatible(t *testing.T) {
	app := newTestApp(t)
	legacy := types.SessionData{
		JWT:       "legacy-token",
		UserName:  "Legacy User",
		Players:   []types.LPSPlayer{{UPlayerID: 1001}},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	actual := decryptTestSession(t, app, encryptTestSession(t, app, &legacy))
	if !reflect.DeepEqual(actual.Workflow, types.SoccerWorkflowState{}) {
		t.Fatalf("legacy session workflow = %#v, want zero value", actual.Workflow)
	}
}
