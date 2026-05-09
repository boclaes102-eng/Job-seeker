package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

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

	r.Get("/api/audit/runs", h.ListAuditRuns)
	r.Get("/api/audit/runs/{runID}", h.GetAuditRun)

	// Serve the React frontend from client/dist (two levels up from server/cmd/server)
	distDir := resolveDist()
	if _, err := os.Stat(distDir); err == nil {
		fs := http.FileServer(http.Dir(distDir))
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			// If the file exists in dist, serve it; otherwise fall back to index.html (SPA)
			path := filepath.Join(distDir, filepath.Clean(strings.TrimPrefix(req.URL.Path, "/")))
			if _, err := os.Stat(path); os.IsNotExist(err) {
				http.ServeFile(w, req, filepath.Join(distDir, "index.html"))
				return
			}
			fs.ServeHTTP(w, req)
		})
	}

	return r
}

// resolveDist finds client/dist relative to the binary's working directory.
func resolveDist() string {
	// go run ./cmd/server runs with cwd = server/
	candidates := []string{
		"../client/dist",
		"../../client/dist",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			abs, _ := filepath.Abs(p)
			return abs
		}
	}
	return "../client/dist"
}
