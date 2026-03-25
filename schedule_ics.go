// ICS calendar event builder with RFC 5545 line folding.
package main

import (
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"
)

type formattedGameEvent struct {
	Description string
	End         time.Time
	ID          string
	Location    string
	Start       time.Time
	Status      string
	Summary     string
}

func buildICS(games []Game) string {
	var builder strings.Builder
	writeICSLine(&builder, "BEGIN:VCALENDAR")
	writeICSLine(&builder, "VERSION:2.0")
	writeICSLine(&builder, "PRODID:-//Craig Johnson Portfolio//Soccer Schedule//EN")
	writeICSLine(&builder, "X-WR-TIMEZONE:"+mountainTimeZoneID)
	for i := range games {
		game := &games[i]
		formatted, ok := canonicalGameEvent(game)
		if !ok {
			log.Printf("skipping game: could not parse start time")
			continue
		}
		writeICSLine(&builder, "BEGIN:VEVENT")
		writeICSLine(&builder, "UID:"+escapeICSText(formatted.ID))
		writeICSLine(&builder, "DTSTAMP:"+time.Now().UTC().Format("20060102T150405Z"))
		writeICSLine(&builder, "DTSTART;TZID="+mountainTimeZoneID+":"+formatted.Start.Format("20060102T150405"))
		writeICSLine(&builder, "DTEND;TZID="+mountainTimeZoneID+":"+formatted.End.Format("20060102T150405"))
		writeICSLine(&builder, "SUMMARY:"+escapeICSText(formatted.Summary))
		writeICSLine(&builder, "DESCRIPTION:"+escapeICSText(formatted.Description))
		writeICSLine(&builder, "LOCATION:"+escapeICSText(formatted.Location))
		writeICSLine(&builder, "STATUS:"+strings.ToUpper(formatted.Status))
		writeICSLine(&builder, "END:VEVENT")
	}
	writeICSLine(&builder, "END:VCALENDAR")
	return builder.String()
}

func canonicalGameEvent(game *Game) (formattedGameEvent, bool) {
	start, end, ok := scheduleTimes(game)
	if !ok {
		return formattedGameEvent{}, false
	}

	start = start.In(mountainTimeLocation)
	end = end.In(mountainTimeLocation)

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
		gameID = fallbackGameID(game)
	}

	status := canonicalGameStatus(game)

	return formattedGameEvent{
		Description: fmt.Sprintf("%s is playing %s\nDivision: %s\nFacility: %s\nField: %s\nResult: %s",
			playerTeam,
			opponentTeam,
			strings.TrimSpace(game.DivisionName),
			strings.TrimSpace(game.FacilityName),
			fieldName,
			strings.TrimSpace(game.Result),
		),
		End:      end,
		ID:       gameID,
		Location: location,
		Start:    start,
		Status:   status,
		Summary:  fmt.Sprintf("%s vs %s - %s", playerTeam, opponentTeam, fieldName),
	}, true
}

func canonicalGameLocation(game *Game) string {
	parts := []string{
		strings.TrimSpace(game.FacilityAddress),
		strings.TrimSpace(game.FacilityCity),
		strings.TrimSpace(game.FacilityState),
		strings.TrimSpace(game.FacilityZIP),
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

func canonicalGameStatus(game *Game) string {
	if strings.EqualFold(strings.TrimSpace(game.Result), "canceled") {
		return "canceled"
	}
	return "confirmed"
}

func escapeICSText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ";", `\;`)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return value
}

func writeICSLine(builder *strings.Builder, line string) {
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
