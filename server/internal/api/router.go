package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/cors"
)

func NewRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"http://localhost:5173", "http://localhost:4173"},
		AllowedMethods: []string{"GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
	})
	r.Use(c.Handler)

	r.Get("/api/jobs", h.ListJobs)
	r.Get("/api/jobs/refresh/stream", h.RefreshJobsStream)
	r.Delete("/api/jobs", h.ResetJobs)
	r.Delete("/api/jobs/new", h.ClearNewJobs)
	r.Patch("/api/jobs/{id}/status", h.UpdateStatus)
	r.Post("/api/jobs/{id}/analyze", h.AnalyzeJob)
	r.Post("/api/jobs/{id}/draft-email", h.DraftEmail)

	r.Get("/api/profile", h.GetProfile)
	r.Put("/api/profile", h.SaveProfile)

	return r
}
