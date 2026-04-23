package schedule

import (
	"testing"
	"time"

	"portfolio/types"
)

func TestParseFlexibleTimeUsesLocalTimezoneForTimezoneLessLayouts(t *testing.T) {
	got, ok := ParseFlexibleTime("2026-01-11T14:55:00")
	if !ok {
		t.Fatal("ParseFlexibleTime returned false")
	}

	want := time.Date(2026, time.January, 11, 14, 55, 0, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("unexpected parsed time: got %v want %v", got, want)
	}
	if got.Location() != time.Local {
		t.Fatalf("unexpected location: got %v want %v", got.Location(), time.Local)
	}
}

func TestParseFlexibleTimePreservesRFC3339Offsets(t *testing.T) {
	got, ok := ParseFlexibleTime("2026-01-11T14:55:00-07:00")
	if !ok {
		t.Fatal("ParseFlexibleTime returned false")
	}

	if got.Format(time.RFC3339) != "2026-01-11T14:55:00-07:00" {
		t.Fatalf("unexpected RFC3339 parse result: %s", got.Format(time.RFC3339))
	}
	if _, offset := got.Zone(); offset != -7*60*60 {
		t.Fatalf("unexpected RFC3339 offset: %d", offset)
	}
}

func TestParseFlexibleTimePreservesUTCForZuluTimestamps(t *testing.T) {
	got, ok := ParseFlexibleTime("2026-01-12T01:00:00.000Z")
	if !ok {
		t.Fatal("ParseFlexibleTime returned false")
	}

	want := time.Date(2026, time.January, 12, 1, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("unexpected parsed time: got %v want %v", got, want)
	}
	if got.Location() != time.UTC {
		t.Fatalf("unexpected location: got %v want %v", got.Location(), time.UTC)
	}
}

func TestParseScheduleTimeTreatsMislabelledZuluTimestampsAsMountainWallTime(t *testing.T) {
	got, ok := ParseScheduleTime("2026-03-29T17:20:00.000Z")
	if !ok {
		t.Fatal("ParseScheduleTime returned false")
	}

	if got.Format(time.RFC3339) != "2026-03-29T17:20:00-06:00" {
		t.Fatalf("unexpected schedule parse result: %s", got.Format(time.RFC3339))
	}
	mtz := MountainTimeLocation
	if got.In(mtz).Format("MST") != "MDT" {
		t.Fatalf("unexpected mountain timezone label: %s", got.In(mtz).Format("MST"))
	}
}

func TestScheduleTimesReturnsFalseForUnparseableStart(t *testing.T) {
	_, _, ok := ScheduleTimes(&types.Game{ID: "no-time"})
	if ok {
		t.Fatal("expected ScheduleTimes to return false for game with no start time")
	}
}
