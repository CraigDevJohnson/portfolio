package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/core"
	"github.com/awslabs/aws-lambda-go-api-proxy/httpadapter"

	"portfolio/internal/app"
	"portfolio/internal/config"
	"portfolio/internal/httpx"
	"portfolio/internal/logging"
	"portfolio/internal/soccerarchive"
)

// lambdaInitializationTimeout bounds Lambda cold-start initialization.
const lambdaInitializationTimeout = 8 * time.Second

type proxyV2 interface {
	ProxyWithContext(ctx context.Context, request events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)
}

type lambdaHandlerFunc func(context.Context, *events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error)

type dailyRunner interface {
	Run(ctx context.Context) (soccerarchive.DailyReport, error)
}

type dailyHandlerFunc func(context.Context, json.RawMessage) (soccerarchive.DailyReport, error)

func initializeDailyLambda(ctx context.Context) (*soccerarchive.DailyWorker, error) {
	limits, err := soccerarchive.LimitsFromEnvironment(os.Getenv)
	if err != nil {
		return nil, err
	}
	baseURL, err := config.NormalizeLPSAPIBaseURL(os.Getenv("LPS_API_BASE_URL"))
	if err != nil {
		return nil, fmt.Errorf("configure LPS source: %w", err)
	}
	store, err := soccerarchive.NewDynamoStore(ctx, os.Getenv(soccerarchive.EnvArchiveTableName), limits)
	if err != nil {
		return nil, fmt.Errorf("configure history store: %w", err)
	}
	worker, err := soccerarchive.NewDailyWorker(store, baseURL, &http.Client{Timeout: 15 * time.Second}, limits, nil)
	if err != nil {
		return nil, err
	}
	return worker, nil
}

func newDailyLambdaHandler(runner dailyRunner) dailyHandlerFunc {
	return func(ctx context.Context, _ json.RawMessage) (soccerarchive.DailyReport, error) {
		report, err := runner.Run(ctx)
		if err != nil {
			slog.Error("soccer_history_daily_failed", slog.Any("error", err), slog.Int("requests", report.Requests), slog.Any("results", report.Results))
			return report, err
		}
		if !report.Complete {
			slog.Warn(soccerarchive.DailyIncompleteLog, slog.Int("requests", report.Requests), slog.Bool("pending_due_work", report.PendingDueWork), slog.Any("results", report.Results))
		} else {
			slog.Info(soccerarchive.DailyCompletedLog, slog.Int("requests", report.Requests), slog.Any("results", report.Results))
		}
		return report, nil
	}
}

func initializeLambda(ctx context.Context) (proxyV2, error) {
	if err := resolveSSMSecrets(ctx); err != nil {
		return nil, fmt.Errorf("resolve SSM secrets: %w", err)
	}
	handler, err := app.NewLambdaHandler(ctx)
	if err != nil {
		return nil, fmt.Errorf("construct application: %w", err)
	}
	return httpadapter.NewV2(withAPIGatewayOrigin(handler)), nil
}

func newLambdaHandler(proxy proxyV2) lambdaHandlerFunc {
	return func(ctx context.Context, request *events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
		if request == nil {
			return events.APIGatewayV2HTTPResponse{StatusCode: http.StatusBadRequest, Body: "invalid request"}, nil
		}
		return proxy.ProxyWithContext(ctx, *request)
	}
}

func withAPIGatewayOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gatewayContext, ok := core.GetAPIGatewayV2ContextFromContext(r.Context())
		domain := strings.TrimSpace(gatewayContext.DomainName)
		if !ok || domain == "" {
			http.Error(w, "gateway request context missing", http.StatusInternalServerError)
			return
		}
		r = httpx.WithTrustedOrigin(r, httpx.TrustedOrigin{Scheme: "https", Host: domain})
		next.ServeHTTP(w, r)
	})
}

func main() {
	if os.Getenv("SOCCER_HISTORY_MODE") == "scheduled" {
		rootLogger, _, warnings := logging.NewLoggerFromEnv()
		slog.SetDefault(rootLogger)
		for _, warning := range warnings {
			rootLogger.Warn("invalid logging configuration; using fallback", slog.String("warning", warning))
		}
		initCtx, cancel := context.WithTimeout(context.Background(), lambdaInitializationTimeout)
		worker, err := initializeDailyLambda(initCtx)
		cancel()
		if err != nil {
			slog.Error("daily history lambda initialization failed", slog.Any("error", err))
			os.Exit(1)
		}
		lambda.Start(newDailyLambdaHandler(worker))
		return
	}
	initCtx, cancel := context.WithTimeout(context.Background(), lambdaInitializationTimeout)
	proxy, err := initializeLambda(initCtx)
	cancel()
	if err != nil {
		slog.Error("lambda initialization failed", slog.Any("error", err))
		os.Exit(1)
	}
	lambda.Start(newLambdaHandler(proxy))
}
