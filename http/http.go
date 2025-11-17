package http

import (
	"admission-module/http/handlers"
	"admission-module/http/middleware"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// SetupRoutes configures all HTTP routes and middleware
func SetupRoutes() {
	// Serve static files with CORS and debug logging
	staticDir := "static"
	absStaticDir, err := filepath.Abs(staticDir)
	if err != nil {
		log.Fatal("Error getting absolute path for static directory:", err)
	}

	http.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		requestedFile := filepath.Join(absStaticDir, strings.TrimPrefix(r.URL.Path, "/static/"))

		if _, err := os.Stat(requestedFile); os.IsNotExist(err) {
			log.Printf("File not found: %s", requestedFile)
			http.NotFound(w, r)
			return
		}

		log.Printf("File found, serving: %s", requestedFile)
		middleware.EnableCORS(func(w http.ResponseWriter, r *http.Request) {
			http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir))).ServeHTTP(w, r)
		})(w, r)
	})

	// API Routes
	http.HandleFunc("/upload-leads", middleware.EnableCORS(handlers.UploadLeads))
	http.HandleFunc("/leads", middleware.EnableCORS(handlers.GetLeads))
	http.HandleFunc("/create-lead", middleware.EnableCORS(handlers.CreateLead))
	// http.HandleFunc("/assign-counsellor", middleware.EnableCORS(handlers.AssignCounsellor))
	http.HandleFunc("/schedule-meet", middleware.EnableCORS(handlers.ScheduleMeet))
	http.HandleFunc("/initiate-payment", middleware.EnableCORS(handlers.InitiatePayment))
	http.HandleFunc("/verify-payment", middleware.EnableCORS(handlers.VerifyPayment))
	http.HandleFunc("/application-action", middleware.EnableCORS(handlers.ApplicationAction))
}
