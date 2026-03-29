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
	internalgoogle "portfolio/internal/google"
	"portfolio/internal/portfolio"
	internalsoccer "portfolio/internal/soccer"
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
	soccerHandler := internalsoccer.NewHandler(&app.Config, app.LPSClient, app.LoginLimiter, app.MountainTZ, soccerGoogleHooks{google: app.GoogleHandler})
	app.GoogleHandler.Soccer = newGoogleSoccerBridge(soccerHandler)

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
	mux.HandleFunc("/soccer", soccerHandler.SoccerPage)
	mux.HandleFunc("/soccer/session", soccerHandler.SessionHandler)
	mux.HandleFunc("/soccer/import", soccerHandler.ImportHandler)
	mux.HandleFunc("/soccer/logout", soccerHandler.LogoutHandler)
	mux.HandleFunc("/soccer/google/add", app.GoogleHandler.AddHandler)
	mux.HandleFunc("/soccer/google/calendar", app.GoogleHandler.CalendarHandler)
	mux.HandleFunc("/soccer/google/connect", app.GoogleHandler.ConnectHandler)
	mux.HandleFunc("/soccer/google/disconnect", app.GoogleHandler.DisconnectHandler)
	mux.HandleFunc("/soccer/fetch", soccerHandler.FetchSchedulesHandler)
	mux.HandleFunc("/soccer/download", soccerHandler.DownloadICSHandler)
	mux.HandleFunc("/soccer/subscribe", soccerHandler.SubscribeHandler)

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
			store, initErr := internalgoogle.NewConnectionStore(initCtx, app.Config.GoogleConnectionTableName)
			if initErr != nil {
				log.Printf("google calendar add disabled: could not initialize connection store: %v", initErr)
				return
			}
			app.GoogleHandler.SetStore(store)
		}()
	}

	select {}
}
