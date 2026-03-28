// Schedule payload construction compatibility wrappers.
package app

import (
	"time"

	internalschedule "portfolio/internal/schedule"
)

const fieldLocationPrefix = internalschedule.FieldLocationPrefix

func mergeGames(base, incoming *Game) Game {
	return internalschedule.MergeGames(base, incoming)
}

func stableGameFields(game *Game) string {
	return internalschedule.StableGameFields(game)
}

func fallbackGameID(game *Game) string {
	return internalschedule.FallbackGameID(game)
}

func gameKey(game *Game) string {
	return internalschedule.GameKey(game)
}

func gameStartTime(game *Game) (time.Time, bool) {
	return internalschedule.GameStartTime(game)
}

func normalizeScheduleGames(games []Game) {
	internalschedule.NormalizeScheduleGames(games)
}

func mergeScheduleGames(games, incoming []Game, indexByKey map[string]int) []Game {
	return internalschedule.MergeScheduleGames(games, incoming, indexByKey)
}

func sortScheduleGames(games []Game) {
	internalschedule.SortScheduleGames(games)
}

func compareScheduleGames(left, right *Game) int {
	return internalschedule.CompareScheduleGames(left, right)
}

func upcomingScheduleGames(games []Game) []Game {
	return internalschedule.UpcomingScheduleGames(games)
}
