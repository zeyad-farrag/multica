package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newRouter returns the Chi router for the team-app server.
//
// AR13 layering reminder: handlers under /api/v1 must remain thin —
// parse → call internal/service/<domain>/ → respond. Handlers MUST NOT
// import pkg/db/queries directly; services own the *db.Queries handle.
//
// pool is reserved for future stories (services hold a *db.Queries
// derived from this pool); this story exposes only /healthz.
func newRouter(pool *pgxpool.Pool) chi.Router {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/api/v1", func(_ chi.Router) {
		// Per-domain mounts land in later stories (Stories 1.5+). The group
		// is reserved here so the URL-versioning contract is visible from day one.
	})

	_ = pool
	return r
}
