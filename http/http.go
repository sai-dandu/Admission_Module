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

	// Lead Management APIs
	http.HandleFunc("/upload-leads", middleware.EnableCORS(handlers.UploadLeads))
	http.HandleFunc("/leads", middleware.EnableCORS(handlers.GetLeads))
	http.HandleFunc("/create-lead", middleware.EnableCORS(handlers.CreateLead))

	// Course Management APIs
	http.HandleFunc("/courses", middleware.EnableCORS(handlers.GetCourses))
	http.HandleFunc("/course", middleware.EnableCORS(handlers.GetCourseByID))
	http.HandleFunc("/create-course", middleware.EnableCORS(handlers.CreateCourse))
	http.HandleFunc("/update-course", middleware.EnableCORS(handlers.UpdateCourse))

	// Payment APIs
	http.HandleFunc("/initiate-payment", middleware.EnableCORS(handlers.InitiatePayment))
	http.HandleFunc("/verify-payment", middleware.EnableCORS(handlers.VerifyPayment))

	// Interview & Application APIs
	http.HandleFunc("/schedule-meet", middleware.EnableCORS(handlers.ScheduleMeet))
	http.HandleFunc("/application-action", middleware.EnableCORS(handlers.ApplicationAction))

	// DLQ Management APIs
	http.HandleFunc("/api/dlq/messages", middleware.EnableCORS(handlers.GetDLQMessages))
	http.HandleFunc("/api/dlq/messages/retry/", middleware.EnableCORS(handlers.RetryDLQMessage))
	http.HandleFunc("/api/dlq/messages/resolve/", middleware.EnableCORS(handlers.ResolveDLQMessage))
	http.HandleFunc("/api/dlq/stats", middleware.EnableCORS(handlers.GetDLQStats))
}
