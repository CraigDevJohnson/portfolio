package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"portfolio/internal/config"
	internalsoccer "portfolio/internal/soccer"
	"portfolio/internal/soccerarchive"
)

// newHistoryStore builds the durable history store from the table name and
// every reviewed limit in the environment. Any missing or invalid value is an
// error, so no runtime enrolls or refreshes teams under an invented ceiling.
func newHistoryStore(ctx context.Context) (*soccerarchive.DynamoStore, soccerarchive.Limits, error) {
	limits, err := soccerarchive.LimitsFromEnvironment(os.Getenv)
	if err != nil {
		return nil, soccerarchive.Limits{}, err
	}
	store, err := soccerarchive.NewDynamoStore(ctx, os.Getenv(soccerarchive.EnvArchiveTableName), limits)
	if err != nil {
		return nil, soccerarchive.Limits{}, fmt.Errorf("configure history store: %w", err)
	}
	return store, limits, nil
}

// initializeSoccerHistory wires durable Soccer history collection once it is
// activated with a table and every reviewed limit, so visitor Team ID lookups
// and granted player imports enroll teams within the admission capacity. An
// incomplete configuration leaves collection off, as before activation,
// without affecting the rest of the site.
func initializeSoccerHistory(ctx context.Context, logger *slog.Logger, soccerHandler *internalsoccer.Handler) {
	if os.Getenv("SOCCER_HISTORY_COLLECTION_ENABLED") != "true" {
		return
	}
	store, limits, err := newHistoryStore(ctx)
	if err != nil {
		logger.Error("soccer history collection left off: incomplete configuration", slog.Any("error", err))
		return
	}
	soccerHandler.SetArchiveStore(store)
	logger.Info("soccer history collection enabled", slog.Int("max_enrolled_teams", limits.MaxEnrolledTeams), slog.Int("reserved_player_slots", limits.ReservedPlayerSlots))
}

// NewDailyHistoryWorker constructs the scheduled history worker for its
// separate Lambda function. It refuses to build without the table, the LPS
// source, and every reviewed limit, so live polling stays off until each is
// set.
func NewDailyHistoryWorker(ctx context.Context) (*soccerarchive.DailyWorker, error) {
	newRootLogger()
	baseURL, err := config.NormalizeLPSAPIBaseURL(os.Getenv("LPS_API_BASE_URL"))
	if err != nil {
		return nil, fmt.Errorf("configure LPS source: %w", err)
	}
	store, limits, err := newHistoryStore(ctx)
	if err != nil {
		return nil, err
	}
	return soccerarchive.NewDailyWorker(store, baseURL, &http.Client{Timeout: lpsClientTimeout}, limits, nil)
}
