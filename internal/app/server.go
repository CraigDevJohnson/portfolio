// Main server setup and routing for the portfolio application.
// This file initializes the HTTP server, sets up routes for both the portfolio pages
// and the soccer scheduling feature, and configures MIME types for static assets.
package app

import (
	"context"
	"log"
	"mime"
	"net"
	"net/http"
	"time"

	"portfolio/internal/config"
	"portfolio/internal/portfolio"
)

// Run loads configuration, constructs the App, registers routes, and starts the server.
func Run() {
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

	cfg := config.Load()
	app := New(cfg)

	mux := http.NewServeMux()

	// portfolio routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		portfolio.HomeHandler(w, r, config.CareerStartYear)
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		portfolio.AboutHandler(w, config.CareerStartYear)
	})
	mux.HandleFunc("/experience", func(w http.ResponseWriter, _ *http.Request) {
		portfolio.ExperienceHandler(w)
	})
	mux.HandleFunc("/experience/timeline", func(w http.ResponseWriter, _ *http.Request) {
		portfolio.ExperienceTimelineHandler(w)
	})
	mux.HandleFunc("/skills", func(w http.ResponseWriter, _ *http.Request) {
		portfolio.SkillsHandler(w)
	})
	mux.HandleFunc("/skills/grid", func(w http.ResponseWriter, _ *http.Request) {
		portfolio.SkillsGridHandler(w)
	})
	mux.HandleFunc("/skills/filtered", func(w http.ResponseWriter, r *http.Request) {
		portfolio.SkillsFilteredHandler(w, r)
	})
	mux.HandleFunc("/skills/detail", func(w http.ResponseWriter, r *http.Request) {
		portfolio.SkillsDetailHandler(w, r)
	})
	mux.HandleFunc("/projects", func(w http.ResponseWriter, _ *http.Request) {
		portfolio.ProjectsHandler(w)
	})
	mux.HandleFunc("/projects/grid", func(w http.ResponseWriter, _ *http.Request) {
		portfolio.ProjectsGridHandler(w)
	})
	mux.HandleFunc("/education", func(w http.ResponseWriter, _ *http.Request) {
		portfolio.EducationHandler(w)
	})
	mux.HandleFunc("/contact", func(w http.ResponseWriter, _ *http.Request) {
		portfolio.ContactHandler(w)
	})

	// soccer routes
	mux.HandleFunc("/soccer", app.soccerHandler)
	mux.HandleFunc("/soccer/session", app.soccerSessionHandler)
	mux.HandleFunc("/soccer/import", app.soccerImportHandler)
	mux.HandleFunc("/soccer/logout", app.soccerLogoutHandler)
	mux.HandleFunc("/soccer/google/add", app.soccerGoogleAddHandler)
	mux.HandleFunc("/soccer/google/calendar", app.soccerGoogleCalendarHandler)
	mux.HandleFunc("/soccer/google/connect", app.soccerGoogleConnectHandler)
	mux.HandleFunc("/soccer/google/disconnect", app.soccerGoogleDisconnectHandler)
	mux.HandleFunc("/soccer/fetch", app.fetchSchedulesHandler)
	mux.HandleFunc("/soccer/download", app.downloadICSHandler)
	mux.HandleFunc("/soccer/subscribe", app.subscribeHandler)

	// static files
	mux.Handle(
		"/static/",
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("cmd/web/static")),
		),
	)

	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
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

	go func() {
		log.Printf("Craig Johnson Portfolio running at %s", config.LocalServerURL(listenAddress))
		log.Fatal(server.Serve(ln))
	}()

	// Initialize the Google connection store in the background so App Runner
	// health checks never wait on AWS SDK startup or credential resolution.
	if app.Config.GoogleEnabled() {
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer initCancel()
			store, initErr := newGoogleConnectionStore(initCtx, &app.Config)
			if initErr != nil {
				log.Printf("google calendar add disabled: could not initialize connection store: %v", initErr)
				return
			}
			app.setGoogleConnectionStore(store)
		}()
	}

	select {}
}
