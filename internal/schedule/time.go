package schedule

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"portfolio/internal/config"
	"portfolio/types"
)

var (
	mountainTimeLocation     *time.Location
	mountainTimeLocationOnce sync.Once
)

// MountainTimeLocation returns the app's canonical timezone for soccer schedules.
func MountainTimeLocation() *time.Location {
	mountainTimeLocationOnce.Do(func() {
		location, err := time.LoadLocation(config.MountainTimeZoneID)
		if err == nil {
			mountainTimeLocation = location
			return
		}

		slog.Default().With(slog.String("component", "schedule")).Warn(
			"could not load configured timezone; falling back to MST",
			slog.String("timezone", config.MountainTimeZoneID),
			slog.Any("error", err),
		)
		const fallbackTimezoneOffsetSeconds = -7 * 60 * 60
		mountainTimeLocation = time.FixedZone("MST", fallbackTimezoneOffsetSeconds)
	})
	return mountainTimeLocation
}

// ParseScheduleTime parses schedule timestamps using the supported LPS formats.
func ParseScheduleTime(value string) (time.Time, bool) {
	if parsed, ok := ParseMislabelledLPSZuluTime(value); ok {
		return parsed, true
	}
	return ParseFlexibleTime(value)
}

// NormalizeLPSScheduleTime converts mislabelled LPS timestamps to RFC3339.
func NormalizeLPSScheduleTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parsed, ok := ParseMislabelledLPSZuluTime(value)
	if !ok {
		return value
	}
	return parsed.Format(time.RFC3339)
}

// ParseMislabelledLPSZuluTime parses local mountain times that incorrectly end in Z.
func ParseMislabelledLPSZuluTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasSuffix(value, "Z") {
		return time.Time{}, false
	}

	trimmed := strings.TrimSuffix(value, "Z")
	for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05"} {
		parsed, err := time.ParseInLocation(layout, trimmed, MountainTimeLocation())
		if err == nil {
			return parsed, true
		}
	}

	return time.Time{}, false
}

// ParseFlexibleTime parses the supported schedule timestamp formats.
func ParseFlexibleTime(value string) (time.Time, bool) {
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
		{layout: "Mon 01/02/06 03:04 PM MST", location: MountainTimeLocation()},
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

// FormatGameDateTime formats a schedule timestamp for display in mountain time.
func FormatGameDateTime(value string) string {
	parsed, ok := ParseScheduleTime(value)
	if !ok {
		return strings.TrimSpace(value)
	}
	return parsed.In(MountainTimeLocation()).Format("Mon 01/02/06 03:04 PM MST")
}

// ScheduleTimes returns the event start and end times for a game.
func ScheduleTimes(game *types.Game) (time.Time, time.Time, bool) {
	start, ok := GameStartTime(game)
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	end, ok := ParseScheduleTime(game.EndAt)
	if !ok || !end.After(start) {
		end = start.Add(config.DefaultGameDuration)
	}
	return start, end, true
}
