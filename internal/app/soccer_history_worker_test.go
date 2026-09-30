package app

import (
	"testing"
)

// reviewedHistoryLimits is a complete reviewed history configuration: the
// table and every numeric limit the runtimes need before they build a store.
var reviewedHistoryLimits = map[string]string{
	"SOCCER_ARCHIVE_TABLE_NAME":      "portfolio-lambda-dev-soccer-history",
	"SOCCER_HISTORY_MAX_TEAMS":       "4",
	"SOCCER_HISTORY_PLAYER_RESERVED": "2",
	"SOCCER_HISTORY_MAX_REQUESTS":    "8",
	"SOCCER_HISTORY_MAX_RETRIES":     "1",
	"SOCCER_HISTORY_MIN_INTERVAL_MS": "250",
}

func TestDailyHistoryWorkerIsBuiltOnlyWithEveryReviewedLimit(t *testing.T) {
	t.Run("every reviewed limit", func(t *testing.T) {
		// Building the worker makes no request: its LPS is a fake and its
		// DynamoDB client points at a closed loopback port.
		lambdaAssemblyEnvironment(t)
		for name, value := range reviewedHistoryLimits {
			t.Setenv(name, value)
		}

		if worker, err := NewDailyHistoryWorker(t.Context()); err != nil || worker == nil {
			t.Fatalf("scheduled worker was not built from a complete reviewed configuration: worker %v, err %v", worker, err)
		}
	})
	for unset := range reviewedHistoryLimits {
		t.Run("without "+unset, func(t *testing.T) {
			lambdaAssemblyEnvironment(t)
			for name, value := range reviewedHistoryLimits {
				t.Setenv(name, value)
			}
			t.Setenv(unset, "")

			if worker, err := NewDailyHistoryWorker(t.Context()); err == nil || worker != nil {
				t.Fatalf("scheduled worker built without %s: %v", unset, err)
			}
		})
	}
}
