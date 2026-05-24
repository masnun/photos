package server

import (
	"net/http"

	"github.com/masnun/photos/admin/handlers"
	"github.com/masnun/photos/admin/manifest"
	"github.com/masnun/photos/admin/storage"
	"github.com/masnun/photos/admin/web"
)

func New(store *manifest.Store, r2 *storage.R2) http.Handler {
	mux := http.NewServeMux()

	ph := &handlers.Photos{Store: store, R2: r2}
	tx := &handlers.Taxonomy{Store: store}

	mux.HandleFunc("GET /api/photos", ph.List)
	mux.HandleFunc("POST /api/photos", ph.Upload)
	mux.HandleFunc("PATCH /api/photos/{id}", ph.Patch)
	mux.HandleFunc("DELETE /api/photos/{id}", ph.Delete)

	mux.HandleFunc("GET /api/genres", tx.ListGenres)
	mux.HandleFunc("POST /api/genres", tx.UpsertGenre)
	mux.HandleFunc("PUT /api/genres/order", tx.ReorderGenres)
	mux.HandleFunc("PUT /api/genres/{slug}/cover", tx.SetGenreCover)
	mux.HandleFunc("PUT /api/genres/{slug}/order", tx.SetGenreOrder)
	mux.HandleFunc("DELETE /api/genres/{slug}", tx.DeleteGenre)

	mux.HandleFunc("GET /api/collections", tx.ListCollections)
	mux.HandleFunc("POST /api/collections", tx.UpsertCollection)
	mux.HandleFunc("PUT /api/collections/{slug}/cover", tx.SetCollectionCover)
	mux.HandleFunc("PUT /api/collections/{slug}/order", tx.SetCollectionOrder)
	mux.HandleFunc("DELETE /api/collections/{slug}", tx.DeleteCollection)

	mux.Handle("/", web.Handler())
	return mux
}
