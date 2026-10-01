package schedule

import (
	"testing"
	"time"

	"portfolio/types"
)

func TestPastGamesWithResults(t *testing.T) {
	pastWithResult := types.Game{ID: "past-with-result", StartAt: mislabelledZuluTime(time.Now().Add(-2 * time.Hour)), Result: "2 - 1"}
	pastWithoutResult := types.Game{ID: "past-without-result", StartAt: mislabelledZuluTime(time.Now().Add(-3 * time.Hour)), Result: "   "}
	futureWithResult := types.Game{ID: "future-with-result", StartAt: mislabelledZuluTime(time.Now().Add(2 * time.Hour)), Result: "1 - 0"}
	unparseableWithResult := types.Game{ID: "unparseable-with-result", StartAt: "", DateTime: "", Result: "3 - 2"}

	games := []types.Game{pastWithResult, pastWithoutResult, futureWithResult, unparseableWithResult}
	filtered := PastGamesWithResults(games)

	if len(filtered) != 1 {
		t.Fatalf("len(PastGamesWithResults) = %d, want 1", len(filtered))
	}
	if filtered[0].ID != "past-with-result" {
		t.Fatalf("filtered[0].ID = %q, want %q", filtered[0].ID, "past-with-result")
	}
}

func mislabelledZuluTime(at time.Time) string {
	return at.In(MountainTimeLocation()).Format("2006-01-02T15:04:05.000") + "Z"
}

func TestNormalizeScheduleGamesNamesAFieldOnlyLocationOnce(t *testing.T) {
	for field, want := range map[string]string{
		"3":            "Field 3",
		"Field 3":      "Field 3",
		"field 3":      "field 3",
		"Fieldhouse B": "Field Fieldhouse B",
	} {
		games := []types.Game{{ID: "game", Field: field}}
		NormalizeScheduleGames(games)
		if games[0].Location != want {
			t.Errorf("field %q: Location = %q, want %q", field, games[0].Location, want)
		}
	}
}
