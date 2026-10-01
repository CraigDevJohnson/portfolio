package schedule

import (
	"fmt"
	"log/slog"
	"regexp"
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

var canonicalResultLine = regexp.MustCompile(`^Result: ((Win|Loss|Draw) \([0-9]+-[0-9]+\)|Canceled|Final)?$`)

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

	playerTeam, opponentTeam := canonicalTeams(game)

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

// canonicalTeams names the team an event is worded for and its opponent.
func canonicalTeams(game *types.Game) (playerTeam, opponentTeam string) {
	playerTeam = strings.TrimSpace(game.PlayerTeamName)
	if playerTeam == "" {
		playerTeam = strings.TrimSpace(game.Home)
	}
	opponentTeam = strings.TrimSpace(game.OpponentTeamName)
	if opponentTeam == "" {
		opponentTeam = strings.TrimSpace(game.Away)
	}
	return playerTeam, opponentTeam
}

// descriptionLineBreak matches one line break in an event description: a
// newline, as Add and the Calendar API write it, or an HTML <br> with any
// newline after it, as Google Calendar's editor saves a description a visitor
// edited there.
var descriptionLineBreak = regexp.MustCompile(`(?i)<br\s*/?>(\r?\n)?|\r?\n`)

// ReplaceCanonicalResult writes the game's result into the result slot of
// the one description block CanonicalGameEvent wrote for it, worded for the
// team the block's heading names: a game between two followed teams reads
// the same whichever team's schedule it now comes from. Everything else in
// the description, including notes before or after the block and each line
// break, whether a newline or an HTML <br>, remains byte for byte. It reports
// false when the description holds no single such block, its result slot or
// another line holds a result this site does not write there, or its heading
// names neither team. A result in the site's own format, or an empty slot, is
// the site's to replace.
func ReplaceCanonicalResult(description string, game *types.Game) (string, bool) {
	lines := descriptionLines(description)
	resultIndex := -1
	for i := 0; i+4 < len(lines); i++ {
		if !strings.Contains(lines[i].text, " is playing ") ||
			!strings.HasPrefix(lines[i+1].text, "Division: ") || !strings.HasPrefix(lines[i+2].text, "Facility: ") ||
			!strings.HasPrefix(lines[i+3].text, "Field: ") || !strings.HasPrefix(lines[i+4].text, "Result: ") {
			continue
		}
		if resultIndex >= 0 {
			return "", false
		}
		resultIndex = i + 4
	}
	if resultIndex < 0 || !canonicalResultLine.MatchString(lines[resultIndex].text) {
		return "", false
	}
	for i, line := range lines {
		if i != resultIndex && strings.HasPrefix(line.text, "Result: ") {
			return "", false
		}
	}
	headingTeam, _, _ := strings.Cut(lines[resultIndex-4].text, " is playing ")
	headingTeam = strings.TrimSpace(headingTeam)
	playerTeam, opponentTeam := canonicalTeams(game)
	if !strings.EqualFold(headingTeam, playerTeam) && !strings.EqualFold(headingTeam, opponentTeam) {
		return "", false
	}
	result := FormatResultLine(ParseGameResult(strings.TrimSpace(game.Result), headingTeam, strings.TrimSpace(game.Home)))
	slot := lines[resultIndex]
	return description[:slot.start] + "Result: " + result + description[slot.start+len(slot.text):], true
}

// descriptionLine is one line of an event description and the byte offset
// where it starts.
type descriptionLine struct {
	text  string
	start int
}

// descriptionLines splits a description at each descriptionLineBreak.
func descriptionLines(description string) []descriptionLine {
	breaks := descriptionLineBreak.FindAllStringIndex(description, -1)
	lines := make([]descriptionLine, 0, len(breaks)+1)
	start := 0
	for _, lineBreak := range breaks {
		lines = append(lines, descriptionLine{text: description[start:lineBreak[0]], start: start})
		start = lineBreak[1]
	}
	return append(lines, descriptionLine{text: description[start:], start: start})
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
	if strings.EqualFold(strings.TrimSpace(game.Result), gameStatusCanceled) {
		return gameStatusCanceled
	}
	return gameStatusConfirmed
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
