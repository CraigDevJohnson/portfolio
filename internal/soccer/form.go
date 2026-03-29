// Package soccer contains soccer-domain helpers and handlers.
package soccer

import (
	"net/url"
	"strconv"
	"strings"
)

// ParseSelectedIDs returns the selected IDs from form values.
func ParseSelectedIDs(form url.Values) map[string]struct{} {
	selectedIDs := make(map[string]struct{})
	for _, id := range form["selected"] {
		id = strings.TrimSpace(id)
		if id != "" {
			selectedIDs[id] = struct{}{}
		}
	}
	return selectedIDs
}

// ParsePlayerIDs returns positive deduplicated player IDs from form values.
func ParsePlayerIDs(values []string) []int {
	seen := make(map[int]struct{})
	playerIDs := make([]int, 0, len(values))
	for _, value := range values {
		playerID, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || playerID <= 0 {
			continue
		}
		if _, exists := seen[playerID]; exists {
			continue
		}
		seen[playerID] = struct{}{}
		playerIDs = append(playerIDs, playerID)
	}
	return playerIDs
}

// ParseTeamIDs returns positive deduplicated team IDs from a delimited string.
func ParseTeamIDs(raw string) []int {
	return ParsePlayerIDs(splitDelimitedValues(raw))
}

// NonEmptyStrings returns trimmed non-empty strings.
func NonEmptyStrings(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			normalized = append(normalized, value)
		}
	}
	return normalized
}

func splitDelimitedValues(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' '
	})
}
