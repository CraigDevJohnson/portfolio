package soccer

import (
	"slices"
	"testing"
	"time"

	"portfolio/cmd/web/partials"
	"portfolio/internal/schedule"
	"portfolio/types"
)

func TestSetTableFragmentGamesOrdersSectionsForDisplay(t *testing.T) {
	now := time.Now()
	games := []types.Game{
		{ID: "past-older", StartAt: mislabelledZuluTime(now.Add(-72 * time.Hour)), Result: "1 - 0"},
		{ID: "past-recent", StartAt: mislabelledZuluTime(now.Add(-24 * time.Hour)), Result: "2 - 1"},
		{ID: "upcoming-soon", StartAt: mislabelledZuluTime(now.Add(24 * time.Hour))},
		{ID: "upcoming-later", StartAt: mislabelledZuluTime(now.Add(72 * time.Hour))},
	}

	var props partials.SoccerTableFragmentProps
	setTableFragmentGames(&props, games)

	if got := gameIDs(props.UpcomingGames); !slices.Equal(got, []string{"upcoming-soon", "upcoming-later"}) {
		t.Fatalf("unexpected upcoming game order: got %#v want %#v", got, []string{"upcoming-soon", "upcoming-later"})
	}
	if got := gameIDs(props.PastGames); !slices.Equal(got, []string{"past-recent", "past-older"}) {
		t.Fatalf("unexpected past game order: got %#v want %#v", got, []string{"past-recent", "past-older"})
	}
}

func gameIDs(games []types.Game) []string {
	ids := make([]string, 0, len(games))
	for i := range games {
		ids = append(ids, games[i].ID)
	}
	return ids
}

func mislabelledZuluTime(at time.Time) string {
	return at.In(schedule.MountainTimeLocation()).Format("2006-01-02T15:04:05.000") + "Z"
}
