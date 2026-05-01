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

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/logging"
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

	cfg := config.Load()
	app := New(&cfg, rootLogger)
	soccerHandler := internalsoccer.NewHandler(
		&app.Config,
		app.LPSClient,
		app.LoginLimiter,
		app.GoogleHandler,
		internalsoccer.NoopSoccerStore{},
		rootLogger.With(slog.String("component", "soccer")),
	)
	app.GoogleHandler.Soccer = soccerHandler

	mux := http.NewServeMux()

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
	mux.HandleFunc("GET /skills/grid", portfolio.SkillsGridHandler)
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
	mux.HandleFunc("GET /soccer/session", soccerHandler.SessionHandler)
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

	// Bind the listener before any slow init so health checks pass immediately.
	listenAddress := config.ServerListenAddress()
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

	// Initialize the Google connection store in the background so App Runner
	// health checks never wait on AWS SDK startup or credential resolution.
	if app.Config.GoogleEnabled() {
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), googleStoreInitTimeout)
			defer initCancel()
			store, initErr := internalgoogle.NewConnectionStore(initCtx, app.Config.GoogleConnectionTableName)
			if initErr != nil {
				app.GoogleHandler.Logger.Warn(
					"google calendar add remains disabled; connection store initialization failed",
					slog.String("table_name", app.Config.GoogleConnectionTableName),
					slog.Any("error", initErr),
				)
				return
			}
			app.GoogleHandler.SetStore(store)
			app.GoogleHandler.Logger.Info(
				"google connection store initialized",
				slog.String("table_name", app.Config.GoogleConnectionTableName),
			)
		}()
	}

	// Initialize the soccer session store in the background — same pattern as Google store.
	if app.Config.SoccerSessionEnabled() {
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), soccerStoreInitTimeout)
			defer initCancel()
			store, initErr := internalsoccer.NewSoccerStore(initCtx, app.Config.SoccerSessionTableName)
			if initErr != nil {
				soccerHandler.Logger.Warn(
					"soccer session store initialization failed; DynamoDB persistence disabled",
					slog.String("table_name", app.Config.SoccerSessionTableName),
					slog.Any("error", initErr),
				)
				return
			}
			soccerHandler.Store = store
			soccerHandler.Logger.Info(
				"soccer session store initialized",
				slog.String("table_name", app.Config.SoccerSessionTableName),
			)
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
