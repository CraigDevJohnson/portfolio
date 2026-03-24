package main

import (
	"context"
	"log"
	"mime"
	"net"
	"net/http"
	"time"
)

func main() {
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

	// routes - pages
	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/about", aboutHandler)
	http.HandleFunc("/experience", experienceHandler)
	http.HandleFunc("/experience/timeline", experienceTimelineHandler)
	http.HandleFunc("/skills", skillsHandler)
	http.HandleFunc("/skills/grid", skillsGridHandler)
	http.HandleFunc("/skills/filtered", skillsFilteredHandler)
	http.HandleFunc("/skills/detail", skillsDetailHandler)
	http.HandleFunc("/projects", projectsHandler)
	http.HandleFunc("/projects/grid", projectsGridHandler)
	http.HandleFunc("/education", educationHandler)
	http.HandleFunc("/contact", contactHandler)

	// soccer routes
	http.HandleFunc("/soccer", soccerHandler)
	http.HandleFunc("/soccer/session", soccerSessionHandler)
	http.HandleFunc("/soccer/import", soccerImportHandler)
	http.HandleFunc("/soccer/logout", soccerLogoutHandler)
	http.HandleFunc("/soccer/google/add", soccerGoogleAddHandler)
	http.HandleFunc("/soccer/google/calendar", soccerGoogleCalendarHandler)
	http.HandleFunc("/soccer/google/connect", soccerGoogleConnectHandler)
	http.HandleFunc("/soccer/google/disconnect", soccerGoogleDisconnectHandler)
	http.HandleFunc("/soccer/fetch", fetchSchedulesHandler)
	http.HandleFunc("/soccer/download", downloadICSHandler)
	http.HandleFunc("/soccer/subscribe", subscribeHandler)

	// static files
	http.Handle(
		"/static/",
		http.StripPrefix("/static/",
			http.FileServer(http.Dir("static")),
		),
	)

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/images/favicon.ico")
	})

	// Bind the listener before any slow init so health checks pass immediately.
	listenAddress := serverListenAddress()
	ln, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", listenAddress, err)
	}

	server := &http.Server{
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Craig Johnson Portfolio running at %s", localServerURL(listenAddress))
		log.Fatal(server.Serve(ln))
	}()

	// Initialize the Google connection store in the background so App Runner
	// health checks never wait on AWS SDK startup or credential resolution.
	if googleEnabled() {
		go func() {
			initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer initCancel()
			store, initErr := newGoogleConnectionStore(initCtx, &configData)
			if initErr != nil {
				log.Printf("google calendar add disabled: could not initialize connection store: %v", initErr)
				return
			}
			setGoogleConnectionStore(store)
		}()
	}

	select {}
}
