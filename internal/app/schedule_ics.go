// ICS calendar event compatibility wrappers.
package app

import (
	"strings"

	internalschedule "portfolio/internal/schedule"
)

type formattedGameEvent = internalschedule.FormattedGameEvent

func buildICS(games []Game) string {
	return internalschedule.BuildICS(games)
}

func canonicalGameEvent(game *Game) (formattedGameEvent, bool) {
	return internalschedule.CanonicalGameEvent(game)
}

func canonicalGameLocation(game *Game) string {
	return internalschedule.CanonicalGameLocation(game)
}

func canonicalGameStatus(game *Game) string {
	return internalschedule.CanonicalGameStatus(game)
}

func escapeICSText(value string) string {
	return internalschedule.EscapeICSText(value)
}

func writeICSLine(builder *strings.Builder, line string) {
	internalschedule.WriteICSLine(builder, line)
}
