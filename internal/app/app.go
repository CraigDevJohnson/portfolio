// Package app provides server startup, route wiring, and dependency injection.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/portal"
	"portfolio/internal/session"
)

// App holds all runtime dependencies, replacing package-level mutable state.
type App struct {
	Config        config.Config
	LPSClient     *http.Client
	LoginLimiter  *session.LoginRateLimiter
	GoogleHandler *internalgoogle.Handler
	PortalHandler *portal.Handler
	Logger        *slog.Logger
}

// New constructs an App with production defaults for the given config.
func New(cfg *config.Config, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}

	app := &App{
		Config:       *cfg,
		LPSClient:    &http.Client{Timeout: lpsClientTimeout},
		LoginLimiter: session.NewLoginRateLimiter(5, time.Minute, config.RateLimiterMaxKeys),
		Logger:       logger.With(slog.String("component", "app")),
	}
	// GoogleHandler starts without a SoccerBridge and is wired after the soccer
	// handler is created.
	app.GoogleHandler = internalgoogle.NewHandler(
		&app.Config,
		app.LPSClient,
		logger.With(slog.String("component", "google")),
		nil,
	)
	if app.Config.PortalEnabled() {
		awsConfig, err := awscfg.LoadDefaultConfig(context.Background(), awscfg.WithRegion(app.Config.PortalAWSRegion))
		if err != nil {
			app.Logger.Warn("portal AWS clients unavailable; portal routes disabled", slog.Any("error", err))
		} else {
			app.PortalHandler = portal.NewHandler(
				&app.Config,
				portal.NewOIDCClient(app.Config.PortalCognitoDomain, app.Config.PortalCognitoClientID, app.Config.PortalCognitoRedirectURI, app.Config.PortalCognitoLogoutURI),
				ec2.NewFromConfig(awsConfig),
				cloudwatch.NewFromConfig(awsConfig),
				cloudwatchlogs.NewFromConfig(awsConfig),
				logger.With(slog.String("component", "portal")),
			)
		}
	}
	return app
}
