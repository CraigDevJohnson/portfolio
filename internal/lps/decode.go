package lps

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"portfolio/types"
)

// DecodeLPSUserPlayers normalizes the LPS /users/check payload into a filtered discovery result.
func DecodeLPSUserPlayers(payload []byte) (UserPlayerDiscovery, error) {
	var discovery UserPlayerDiscovery

	var envelope UserCheckResponse
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return discovery, errors.New("The player lookup response format was not recognized.")
	}
	if envelope.AuthFailure {
		message := strings.TrimSpace(envelope.Error)
		if message == "" {
			message = "Let's Play Soccer rejected the imported token."
		}
		return discovery, NewFetchError(ErrorUnauthorized, 0, http.StatusUnauthorized, "%s", message)
	}

	discovery.UserName = fullName(strings.TrimSpace(envelope.FirstName), strings.TrimSpace(envelope.LastName))
	if discovery.UserName == "" {
		discovery.UserName = "Let's Play Soccer account"
	}
	if len(envelope.Players) == 0 {
		discovery.Players = []types.LPSPlayer{}
		return discovery, nil
	}

	deletedPlayerIDs := make(map[int]struct{})
	for _, userPlayer := range envelope.UserPlayers {
		if userPlayer.Deleted {
			deletedPlayerIDs[userPlayer.PlayerID] = struct{}{}
		}
	}
	if len(deletedPlayerIDs) == 0 {
		discovery.Players = envelope.Players
		return discovery, nil
	}

	players := make([]types.LPSPlayer, 0, len(envelope.Players))
	for _, player := range envelope.Players {
		if _, deleted := deletedPlayerIDs[player.UPlayerID]; deleted {
			continue
		}
		players = append(players, player)
	}

	discovery.Players = players
	return discovery, nil
}
