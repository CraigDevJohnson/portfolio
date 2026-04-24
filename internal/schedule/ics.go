package schedule

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"portfolio/internal/config"
	"portfolio/types"
)

type FormattedGameEvent struct {
	Description string
	End         time.Time
	ID          string
	Location    string
	Start       time.Time
	Status      string
	Summary     string
}

// BuildICS renders the provided games as an iCalendar payload.
func BuildICS(games []types.Game) string {
	var builder strings.Builder
	WriteICSLine(&builder, "BEGIN:VCALENDAR")
	WriteICSLine(&builder, "VERSION:2.0")
	WriteICSLine(&builder, "PRODID:-//Craig Johnson Portfolio//Soccer Schedule//EN")
	WriteICSLine(&builder, "X-WR-TIMEZONE:"+config.MountainTimeZoneID)
	for i := range games {
		game := &games[i]
		formatted, ok := CanonicalGameEvent(game)
		if !ok {
			slog.Default().With(slog.String("component", "schedule")).Warn(
				"skipping game during ICS build; could not parse start time",
				slog.String("game_id", game.ID),
				slog.String("start_at", game.StartAt),
			)
			continue
		}
		WriteICSLine(&builder, "BEGIN:VEVENT")
		WriteICSLine(&builder, "UID:"+EscapeICSText(formatted.ID))
		WriteICSLine(&builder, "DTSTAMP:"+time.Now().UTC().Format("20060102T150405Z"))
		WriteICSLine(&builder, "DTSTART;TZID="+config.MountainTimeZoneID+":"+formatted.Start.Format("20060102T150405"))
		WriteICSLine(&builder, "DTEND;TZID="+config.MountainTimeZoneID+":"+formatted.End.Format("20060102T150405"))
		WriteICSLine(&builder, "SUMMARY:"+EscapeICSText(formatted.Summary))
		WriteICSLine(&builder, "DESCRIPTION:"+EscapeICSText(formatted.Description))
		WriteICSLine(&builder, "LOCATION:"+EscapeICSText(formatted.Location))
		WriteICSLine(&builder, "STATUS:"+strings.ToUpper(formatted.Status))
		WriteICSLine(&builder, "END:VEVENT")
	}
	WriteICSLine(&builder, "END:VCALENDAR")
	return builder.String()
}

// CanonicalGameEvent converts a game into the normalized event shape used for ICS output.
func CanonicalGameEvent(game *types.Game) (FormattedGameEvent, bool) {
	start, end, ok := ScheduleTimes(game)
	if !ok {
		return FormattedGameEvent{}, false
	}

	start = start.In(MountainTimeLocation())
	end = end.In(MountainTimeLocation())

	playerTeam := strings.TrimSpace(game.PlayerTeamName)
	if playerTeam == "" {
		playerTeam = strings.TrimSpace(game.Home)
	}

	opponentTeam := strings.TrimSpace(game.OpponentTeamName)
	if opponentTeam == "" {
		opponentTeam = strings.TrimSpace(game.Away)
	}

	fieldName := strings.TrimSpace(game.Field)
	location := canonicalGameLocation(game)
	if location == "" {
		location = strings.TrimSpace(game.Location)
	}

	gameID := strings.TrimSpace(game.ID)
	if gameID == "" {
		gameID = FallbackGameID(game)
	}

	status := canonicalGameStatus(game)
	formattedResult := FormatResultLine(ParseGameResult(strings.TrimSpace(game.Result), playerTeam, strings.TrimSpace(game.Home)))

	return FormattedGameEvent{
		Description: fmt.Sprintf("%s is playing %s\nDivision: %s\nFacility: %s\nField: %s\nResult: %s",
			playerTeam,
			opponentTeam,
			strings.TrimSpace(game.DivisionName),
			gameFacilityName(game),
			fieldName,
			formattedResult,
		),
		End:      end,
		ID:       gameID,
		Location: location,
		Start:    start,
		Status:   status,
		Summary:  fmt.Sprintf("%s vs %s - %s", playerTeam, opponentTeam, fieldName),
	}, true
}

func canonicalGameLocation(game *types.Game) string {
	if game == nil || game.Facility == nil {
		return ""
	}

	parts := []string{
		strings.TrimSpace(game.Facility.Address),
		strings.TrimSpace(game.Facility.City),
		strings.TrimSpace(game.Facility.State),
		strings.TrimSpace(game.Facility.ZIP),
	}

	locationParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		locationParts = append(locationParts, part)
	}

	return strings.Join(locationParts, ", ")
}

func canonicalGameStatus(game *types.Game) string {
	if strings.EqualFold(strings.TrimSpace(game.Result), "canceled") {
		return "canceled"
	}
	return "confirmed"
}

// EscapeICSText escapes reserved characters for ICS text fields.
func EscapeICSText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ";", `\;`)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

// WriteICSLine writes an ICS line using RFC-5545 line folding.
func WriteICSLine(builder *strings.Builder, line string) {
	const maxLineBytes = 75

	firstSegment := true
	for line != "" {
		available := maxLineBytes
		if !firstSegment {
			builder.WriteByte(' ')
			available--
		}

		written := 0
		for index := 0; index < len(line); {
			_, size := utf8.DecodeRuneInString(line[index:])
			if written > 0 && written+size > available {
				break
			}
			written += size
			index += size
		}

		builder.WriteString(line[:written])
		builder.WriteString("\r\n")
		line = line[written:]
		firstSegment = false
	}
}
