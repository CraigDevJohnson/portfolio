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
	}

	encrypted := encryptTestSession(t, app, &expected)
	actual := decryptTestSession(t, app, encrypted)

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("decryptSession mismatch: got %#v want %#v", actual, expected)
	}
}
