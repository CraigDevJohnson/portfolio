package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
)

// runtimeLogs collects the JSON log records a runtime writes to its log
// group, which the history metric filters and alarms read.
type runtimeLogs struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (logs *runtimeLogs) Write(p []byte) (int, error) {
	logs.mu.Lock()
	defer logs.mu.Unlock()
	return logs.buffer.Write(p)
}

// logger returns a JSON logger that writes to the capture.
func (logs *runtimeLogs) logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(logs, nil))
}

// includeDefaultLogger also sends the process default logger to the capture
// for the rest of the test, as the runtime's log group also receives it.
func (logs *runtimeLogs) includeDefaultLogger(t *testing.T) {
	t.Helper()
	previous := slog.Default()
	slog.SetDefault(logs.logger())
	t.Cleanup(func() { slog.SetDefault(previous) })
}

// take returns the records written since the last take.
func (logs *runtimeLogs) take(t *testing.T) []map[string]any {
	t.Helper()
	logs.mu.Lock()
	defer logs.mu.Unlock()
	records := make([]map[string]any, 0)
	for _, line := range bytes.Split(bytes.TrimSpace(logs.buffer.Bytes()), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("log line %q is not JSON: %v", line, err)
		}
		records = append(records, record)
	}
	logs.buffer.Reset()
	return records
}

// admissionAlarmRecords returns the records the admission alarm counts and
// any ERROR records among records.
func admissionAlarmRecords(records []map[string]any) (rejections, errorRecords []map[string]any) {
	for _, record := range records {
		if record["msg"] == "soccer_history_admission_rejected" {
			rejections = append(rejections, record)
		}
		if record["level"] == "ERROR" {
			errorRecords = append(errorRecords, record)
		}
	}
	return rejections, errorRecords
}

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
