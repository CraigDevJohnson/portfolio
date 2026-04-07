package app

import (
	"context"
	"errors"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"portfolio/internal/config"
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/portfolio"
	internalsoccer "portfolio/internal/soccer"
)

const serverShutdownTimeout = 10 * time.Second

func registerMIMETypes() {
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
			log.Fatalf("Failed to add MIME type for %s: %v", ext, err)
		}
	}
}

// Run loads configuration, constructs the App, registers routes, and starts the server.
func Run() {
	registerMIMETypes()

	cfg := config.Load()
	app := New(&cfg)
	soccerHandler := internalsoccer.NewHandler(&app.Config, app.LPSClient, app.LoginLimiter, app.GoogleHandler)
	app.GoogleHandler.Soccer = soccerHandler

	mux := http.NewServeMux()

	// portfolio routes
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		portfolio.HomeHandler(w, r, config.CareerStartYear)
	})
	mux.HandleFunc("GET /about", func(w http.ResponseWriter, r *http.Request) {
		portfolio.AboutHandler(w, r, config.CareerStartYear)
	})
	mux.HandleFunc("GET /experience", portfolio.ExperienceHandler)
	mux.HandleFunc("GET /experience/timeline", portfolio.ExperienceTimelineHandler)
	mux.HandleFunc("GET /skills", portfolio.SkillsHandler)
	mux.HandleFunc("GET /skills/grid", portfolio.SkillsGridHandler)
	mux.HandleFunc("GET /skills/filtered", portfolio.SkillsFilteredHandler)
	mux.HandleFunc("GET /skills/detail", portfolio.SkillsDetailHandler)
	mux.HandleFunc("GET /projects", portfolio.ProjectsHandler)
	mux.HandleFunc("GET /projects/grid", portfolio.ProjectsGridHandler)
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
	mux.HandleFunc("POST /soccer/google/calendar", app.GoogleHandler.CalendarHandler)
	mux.HandleFunc("GET /soccer/google/connect", app.GoogleHandler.ConnectHandler)
	mux.HandleFunc("POST /soccer/google/disconnect", app.GoogleHandler.DisconnectHandler)
	mux.HandleFunc("POST /soccer/fetch", soccerHandler.FetchSchedulesHandler)
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
		log.Fatalf("failed to listen on %s: %v", listenAddress, err)
	}

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	serveErrCh := make(chan error, 1)

	go func() {
		log.Printf("Craig Johnson Portfolio running at %s", config.LocalServerURL(listenAddress))
		if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
		}
	}()

	// Initialize the Google connection store in the background so App Runner
	// health checks never wait on AWS SDK startup or credential resolution.
	if app.Config.GoogleEnabled() {
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer initCancel()
			store, initErr := internalgoogle.NewConnectionStore(initCtx, app.Config.GoogleConnectionTableName)
			if initErr != nil {
				log.Printf("google calendar add disabled: could not initialize connection store: %v", initErr)
				return
			}
			app.GoogleHandler.SetStore(store)
		}()
	}

	shutdownSignals, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serveErrCh:
		stopSignals()
		log.Fatalf("server stopped unexpectedly: %v", err)
	case <-shutdownSignals.Done():
		stopSignals()
		log.Printf("shutting down server")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown failed: %v", err)
	}
	app.LoginLimiter.Close()
}

func isGoogleCallbackRequest(r *http.Request) bool {
	query := r.URL.Query()
	return query.Get("code") != "" || query.Get("error") != "" || query.Get("state") != ""
}
