package soccer

import (
	"net/url"
	"strconv"
	"strings"
)

func parseSelectedIDs(form url.Values) map[string]struct{} {
	selectedIDs := make(map[string]struct{})
	for _, id := range form["selected"] {
		id = strings.TrimSpace(id)
		if id != "" {
			selectedIDs[id] = struct{}{}
		}
	}
	return selectedIDs
}

func parsePlayerIDs(values []string) []int {
	return parsePositiveUniqueIDs(values)
}

func parseTeamIDs(raw string) []int {
	return parsePositiveUniqueIDs(splitDelimitedValues(raw))
}

func hasInvalidPlayerInput(rawValues []string, playerIDs []int) bool {
	if len(playerIDs) > 0 {
		return false
	}

	return hasNonEmptyTrimmedValues(rawValues)
}

// scheduleFormInput holds the parsed form values shared by schedule handlers.
type scheduleFormInput struct {
	TeamCodes    string
	RawPlayerIDs []string
	PlayerIDs    []int
}

func parseScheduleFormInput(form url.Values) scheduleFormInput {
	rawPlayerIDs := form["player_ids"]
	return scheduleFormInput{
		TeamCodes:    form.Get("team_codes"),
		RawPlayerIDs: rawPlayerIDs,
		PlayerIDs:    parsePlayerIDs(rawPlayerIDs),
	}
}

func parsePositiveUniqueIDs(values []string) []int {
	seen := make(map[int]struct{})
	ids := make([]int, 0, len(values))
	for _, value := range values {
		id, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func hasNonEmptyTrimmedValues(rawValues []string) bool {
	for _, value := range rawValues {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}

	return false
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
