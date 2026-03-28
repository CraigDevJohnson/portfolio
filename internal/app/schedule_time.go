// Time parsing compatibility wrappers for Mountain timezone schedules.
package app

import (
	"time"

	internalschedule "portfolio/internal/schedule"
)

func loadMountainTimeLocation() *time.Location {
	return internalschedule.MountainTimeLocation
}

func parseScheduleTime(value string) (time.Time, bool) {
	return internalschedule.ParseScheduleTime(value)
}

func normalizeLPSScheduleTime(value string) string {
	return internalschedule.NormalizeLPSScheduleTime(value)
}

func parseMislabelledLPSZuluTime(value string) (time.Time, bool) {
	return internalschedule.ParseMislabelledLPSZuluTime(value)
}

func parseFlexibleTime(value string) (time.Time, bool) {
	return internalschedule.ParseFlexibleTime(value)
}

func formatGameDateTime(value string) string {
	return internalschedule.FormatGameDateTime(value)
}

func scheduleTimes(game *Game) (time.Time, time.Time, bool) {
	return internalschedule.ScheduleTimes(game)
}
