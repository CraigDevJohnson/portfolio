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

	// routes - pages
	mux.HandleFunc("/", app.homeHandler)
	mux.HandleFunc("/about", app.aboutHandler)
	mux.HandleFunc("/experience", app.experienceHandler)
	mux.HandleFunc("/experience/timeline", app.experienceTimelineHandler)
	mux.HandleFunc("/skills", app.skillsHandler)
	mux.HandleFunc("/skills/grid", app.skillsGridHandler)
	mux.HandleFunc("/skills/filtered", app.skillsFilteredHandler)
	mux.HandleFunc("/skills/detail", app.skillsDetailHandler)
	mux.HandleFunc("/projects", app.projectsHandler)
	mux.HandleFunc("/projects/grid", app.projectsGridHandler)
	mux.HandleFunc("/education", app.educationHandler)
	mux.HandleFunc("/contact", app.contactHandler)

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
