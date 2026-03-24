package main

import (
	"log"
	"strings"
	"time"
)

func loadMountainTimeLocation() *time.Location {
	location, err := time.LoadLocation(mountainTimeZoneID)
	if err == nil {
		return location
	}

	log.Printf("could not load %s timezone; falling back to MST: %v", mountainTimeZoneID, err)
	return time.FixedZone("MST", -7*60*60)
}

func parseScheduleTime(value string) (time.Time, bool) {
	if parsed, ok := parseMislabelledLPSZuluTime(value); ok {
		return parsed, true
	}
	return parseFlexibleTime(value)
}

func normalizeLPSScheduleTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parsed, ok := parseMislabelledLPSZuluTime(value)
	if !ok {
		return value
	}
	return parsed.Format(time.RFC3339)
}

func parseMislabelledLPSZuluTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasSuffix(value, "Z") {
		return time.Time{}, false
	}

	trimmed := strings.TrimSuffix(value, "Z")
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
		parsed, err := time.ParseInLocation(layout, trimmed, mountainTimeLocation)
		if err == nil {
			return parsed, true
		}
	}

	return time.Time{}, false
}

func parseFlexibleTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	layouts := []struct {
		layout   string
		location *time.Location
	}{
		{layout: time.RFC3339Nano},
		{layout: time.RFC3339},
		{layout: "2006-01-02T15:04:05.000Z", location: time.UTC},
		{layout: "2006-01-02T15:04:05Z", location: time.UTC},
		{layout: "2006-01-02T15:04:05", location: time.Local},
		{layout: "2006-01-02 15:04:05", location: time.Local},
		{layout: "2006-01-02 15:04", location: time.Local},
		{layout: "Mon 01/02/06 03:04 PM MST", location: mountainTimeLocation},
		{layout: "Mon 01/02/06 03:04 PM", location: time.Local},
	}
	for _, candidate := range layouts {
		var (
			parsed time.Time
			err    error
		)
		if candidate.location != nil {
			parsed, err = time.ParseInLocation(candidate.layout, value, candidate.location)
		} else {
			parsed, err = time.Parse(candidate.layout, value)
		}
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func formatGameDateTime(value string) string {
	parsed, ok := parseScheduleTime(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return parsed.In(mountainTimeLocation).Format("Mon 01/02/06 03:04 PM MST")
}

func scheduleTimes(game *Game) (time.Time, time.Time, bool) {
	start, ok := gameStartTime(game)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	end, ok := parseScheduleTime(game.EndAt)
	if !ok || !end.After(start) {
		end = start.Add(defaultGameDuration)
	}
	return start, end, true
}
