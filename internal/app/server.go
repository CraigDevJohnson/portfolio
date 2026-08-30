package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"portfolio/internal/buildinfo"
	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/logging"
	"portfolio/internal/portal"
	"portfolio/internal/portfolio"
	internalsoccer "portfolio/internal/soccer"
)

const serverShutdownTimeout = 10 * time.Second

const (
	lpsClientTimeout       = 15 * time.Second
	httpServerReadTimeout  = 15 * time.Second
	httpServerWriteTimeout = 60 * time.Second
	httpServerIdleTimeout  = 60 * time.Second
	googleStoreInitTimeout = 10 * time.Second
	soccerStoreInitTimeout = 10 * time.Second
)

func registerMIMETypes() error {
	mimeTypes := map[string]string{
		".css":  "text/css",
		".js":   "application/javascript",
		".ico":  "image/x-icon",
		".svg":  "image/svg+xml",
		".webp": "image/webp",
		".png":  "image/png",
		".jpg":  "image/jpeg",
	}
	for ext, mtype := range mimeTypes {
		if err := mime.AddExtensionType(ext, mtype); err != nil {
			return fmt.Errorf("add MIME type for %s: %w", ext, err)
		}
	}
	return nil
}

func buildMux(app *App, rootLogger *slog.Logger, localPortalPreview bool) (*http.ServeMux, *internalsoccer.Handler) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler(buildinfo.Revision()))

	soccerHandler := internalsoccer.NewHandler(
		&app.Config,
		app.LPSClient,
		app.LoginLimiter,
		app.GoogleHandler,
		internalsoccer.NoopSoccerStore{},
		rootLogger.With(slog.String("component", "soccer")),
	)
	app.GoogleHandler.Soccer = soccerHandler

	// portfolio routes
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		portfolio.HomeHandler(w, r, config.CareerStartYear)
	})
	mux.HandleFunc("GET /home", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /about", func(w http.ResponseWriter, r *http.Request) {
		portfolio.AboutHandler(w, r, config.CareerStartYear)
	})
	mux.HandleFunc("GET /experience", portfolio.ExperienceHandler)
	mux.HandleFunc("GET /skills", portfolio.SkillsHandler)
	mux.HandleFunc("GET /skills/filtered", portfolio.SkillsFilteredHandler)
	mux.HandleFunc("GET /skills/detail", portfolio.SkillsDetailHandler)
	mux.HandleFunc("GET /projects", portfolio.ProjectsHandler)
	mux.HandleFunc("GET /education", portfolio.EducationHandler)
	mux.HandleFunc("GET /contact", portfolio.ContactHandler)

	// soccer routes
	mux.HandleFunc("/soccer", func(w http.ResponseWriter, r *http.Request) {
		if isGoogleCallbackRequest(r) {
			app.GoogleHandler.CallbackHandler(w, r)
			return
		}
		soccerHandler.SoccerPage(w, r)
	})
	mux.HandleFunc("POST /soccer/import", soccerHandler.ImportHandler)
	mux.HandleFunc("POST /soccer/logout", soccerHandler.LogoutHandler)
	mux.HandleFunc("POST /soccer/google/add", app.GoogleHandler.AddHandler)
	mux.HandleFunc("POST /soccer/google/sync-results", app.GoogleHandler.SyncResultsHandler)
	mux.HandleFunc("POST /soccer/google/calendar", app.GoogleHandler.CalendarHandler)
	mux.HandleFunc("GET /soccer/google/connect", app.GoogleHandler.ConnectHandler)
	mux.HandleFunc("POST /soccer/google/disconnect", app.GoogleHandler.DisconnectHandler)
	mux.HandleFunc("POST /soccer/fetch", soccerHandler.FetchSchedulesHandler)
	mux.HandleFunc("POST /soccer/discover-teams", soccerHandler.DiscoverTeamsHandler)
	mux.HandleFunc("POST /soccer/download", soccerHandler.DownloadICSHandler)

	// portal routes
	if localPortalPreview {
		mux.HandleFunc("GET /__preview/soccer/{fixture}", soccerPreviewPageHandler)
		mux.HandleFunc("POST /__preview/soccer/download", soccerPreviewDownloadHandler)
		ph := portal.NewPreviewHandler(rootLogger.With(slog.String("component", "portal_preview")))
		mux.HandleFunc("GET /__preview/portal/error", ph.ErrorPageHandler)
		mux.HandleFunc("GET /login", ph.RedirectToDashboardHandler)
		mux.HandleFunc("GET /auth/callback", ph.RedirectToDashboardHandler)
		mux.HandleFunc("POST /logout", ph.RedirectToDashboardHandler)
		mux.HandleFunc("GET /mgmt", ph.DashboardHandler)
		mux.HandleFunc("POST /mgmt/instances/{id}/start", ph.InstanceActionHandler)
		mux.HandleFunc("POST /mgmt/instances/{id}/stop", ph.InstanceActionHandler)
		mux.HandleFunc("POST /mgmt/instances/{id}/restart", ph.InstanceActionHandler)
		mux.HandleFunc("GET /mgmt/instances/{id}/metrics", ph.MetricsHandler)
		mux.HandleFunc("GET /mgmt/instances/{id}/logs", ph.LogsHandler)
	} else if app.Config.PortalEnabled() && app.PortalHandler != nil {
		ph := app.PortalHandler
		mux.HandleFunc("GET /login", ph.LoginPageHandler)
		mux.HandleFunc("GET /auth/callback", ph.CallbackHandler)
		mux.HandleFunc("POST /logout", ph.LogoutHandler)
		mux.HandleFunc("GET /mgmt", ph.RequireAuth(ph.DashboardHandler))
		mux.HandleFunc("POST /mgmt/instances/{id}/start", ph.RequireAuth(ph.InstanceActionHandler))
		mux.HandleFunc("POST /mgmt/instances/{id}/stop", ph.RequireAuth(ph.InstanceActionHandler))
		mux.HandleFunc("POST /mgmt/instances/{id}/restart", ph.RequireAuth(ph.InstanceActionHandler))
		mux.HandleFunc("GET /mgmt/instances/{id}/metrics", ph.RequireAuth(ph.MetricsHandler))
		mux.HandleFunc("GET /mgmt/instances/{id}/logs", ph.RequireAuth(ph.LogsHandler))
	}

	// static files
	mux.Handle(
		"/static/",
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("cmd/web/static")),
		),
	)

	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		http.ServeFile(w, r, "cmd/web/static/images/favicon.ico")
	})

	return mux, soccerHandler
}

func initializeGoogleStore(ctx context.Context, app *App) error {
	store, initErr := internalgoogle.NewConnectionStore(ctx, app.Config.GoogleConnectionTableName)
	if initErr != nil {
		return initErr
	}
	app.GoogleHandler.SetStore(store)
	app.GoogleHandler.Logger.Info(
		"google connection store initialized",
		slog.String("table_name", app.Config.GoogleConnectionTableName),
	)
	return nil
}

func initializeSoccerStore(ctx context.Context, app *App, soccerHandler *internalsoccer.Handler) error {
	store, initErr := internalsoccer.NewSoccerStore(ctx, app.Config.SoccerSessionTableName)
	if initErr != nil {
		return initErr
	}
	soccerHandler.SetStore(store)
	soccerHandler.Logger.Info(
		"soccer session store initialized",
		slog.String("table_name", app.Config.SoccerSessionTableName),
	)
	return nil
}

// NewLambdaHandler constructs the HTTP handler for Lambda + API Gateway deployments.
func NewLambdaHandler(ctx context.Context) (http.Handler, error) {
	rootLogger, _, warnings := logging.NewLoggerFromEnv()
	slog.SetDefault(rootLogger)
	for _, warning := range warnings {
		rootLogger.Warn("invalid logging configuration; using fallback", slog.String("warning", warning))
	}

	if err := registerMIMETypes(); err != nil {
		rootLogger.Error("mime type registration failed", slog.Any("error", err))
		return nil, err
	}

	cfg := config.Load()
	app := New(&cfg, rootLogger)
	mux, soccerHandler := buildMux(app, rootLogger, false)

	if app.Config.GoogleEnabled() {
		if err := initializeGoogleStore(ctx, app); err != nil {
			return nil, fmt.Errorf("initialize Google connection store: %w", err)
		}
	}
	if app.Config.SoccerSessionEnabled() {
		if err := initializeSoccerStore(ctx, app, soccerHandler); err != nil {
			return nil, fmt.Errorf("initialize soccer session store: %w", err)
		}
	}

	return withRequestLogging(rootLogger.With(slog.String("component", "http")), mux), nil
}

// Run loads configuration, constructs the App, registers routes, and starts the server.
func Run() error {
	rootLogger, logConfig, warnings := logging.NewLoggerFromEnv()
	slog.SetDefault(rootLogger)
	for _, warning := range warnings {
		rootLogger.Warn("invalid logging configuration; using fallback", slog.String("warning", warning))
	}

	appLogger := rootLogger.With(slog.String("component", "app"))

	if err := registerMIMETypes(); err != nil {
		appLogger.Error("mime type registration failed", slog.Any("error", err))
		return err
	}

	listenAddress := config.ServerListenAddress()
	localPortalPreview := config.LocalPortalPreviewEnabled(listenAddress)
	if config.LocalPortalPreviewRequested() && !localPortalPreview {
		appLogger.Warn(
			"local portal preview refused; the server must bind to a loopback address",
			slog.String("listen_address", listenAddress),
		)
	}

	cfg := config.Load()
	if localPortalPreview {
		// Preview mode must never initialize the real portal's Cognito or AWS
		// dependencies, even when live portal variables are also present locally.
		cfg.PortalSessionKey = nil
		cfg.PortalCognitoDomain = ""
		cfg.PortalCognitoClientID = ""
		cfg.PortalCognitoRedirectURI = ""
		cfg.PortalCognitoLogoutURI = ""
		appLogger.Warn(
			"local portal preview enabled; mock data only and no AWS actions will be sent",
			slog.String("preview_url", config.LocalServerURL(listenAddress)+"/mgmt"),
		)
	}
	app := New(&cfg, rootLogger)
	mux, soccerHandler := buildMux(app, rootLogger, localPortalPreview)

	// Bind the listener before any slow init so health checks pass immediately.
	ln, err := net.Listen("tcp", listenAddress)
	if err != nil {
		appLogger.Error("failed to listen", slog.String("listen_address", listenAddress), slog.Any("error", err))
		return err
	}

	server := &http.Server{
		Handler:     withRequestLogging(rootLogger.With(slog.String("component", "http")), mux),
		ReadTimeout: httpServerReadTimeout,
		// 60s covers sync-results, which makes N sequential Google Calendar API
		// calls before writing its response (21 games ≈ 15s in practice).
		WriteTimeout: httpServerWriteTimeout,
		IdleTimeout:  httpServerIdleTimeout,
	}
	serveErrCh := make(chan error, 1)

	go func() {
		appLogger.Info(
			"server listening",
			slog.String("listen_address", listenAddress),
			slog.String("local_url", config.LocalServerURL(listenAddress)),
			slog.String("log_format", logConfig.Format),
			slog.String("log_level", logConfig.Level.String()),
			slog.Bool("log_add_source", logConfig.AddSource),
		)
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
		}
	}()

	// Initialize the Google connection store in the background so server startup
	// and health checks never wait on AWS SDK startup or credential resolution.
	if !localPortalPreview && app.Config.GoogleEnabled() {
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), googleStoreInitTimeout)
			defer initCancel()
			if err := initializeGoogleStore(initCtx, app); err != nil {
				app.GoogleHandler.Logger.Warn(
					"google calendar add remains disabled; connection store initialization failed",
					slog.String("table_name", app.Config.GoogleConnectionTableName),
					slog.Any("error", err),
				)
			}
		}()
	}

	// Initialize the soccer session store in the background — same pattern as Google store.
	if !localPortalPreview && app.Config.SoccerSessionEnabled() {
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), soccerStoreInitTimeout)
			defer initCancel()
			if err := initializeSoccerStore(initCtx, app, soccerHandler); err != nil {
				soccerHandler.Logger.Warn(
					"soccer session store initialization failed; DynamoDB persistence disabled",
					slog.String("table_name", app.Config.SoccerSessionTableName),
					slog.Any("error", err),
				)
			}
		}()
	}

	shutdownSignals, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serveErrCh:
		stopSignals()
		appLogger.Error("server stopped unexpectedly", slog.Any("error", err))
		return err
	case <-shutdownSignals.Done():
		stopSignals()
		appLogger.Info("shutting down server")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		appLogger.Error("server shutdown failed", slog.Any("error", err))
		return err
	}
	app.LoginLimiter.Close()
	appLogger.Info("server shutdown complete")
	return nil
}

func isGoogleCallbackRequest(r *http.Request) bool {
	query := r.URL.Query()
	return query.Get("code") != "" || query.Get("error") != "" || query.Get("state") != ""
}
