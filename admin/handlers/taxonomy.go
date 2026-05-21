package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/masnun/photos/admin/manifest"
)

type Taxonomy struct {
	Store *manifest.Store
}

func (h *Taxonomy) ListGenres(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Store.Snapshot().Genres)
}

func (h *Taxonomy) UpsertGenre(w http.ResponseWriter, r *http.Request) {
	var g manifest.Genre
	if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if g.Slug == "" || g.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("slug and name required"))
		return
	}
	if err := h.Store.UpsertGenre(g); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *Taxonomy) DeleteGenre(w http.ResponseWriter, r *http.Request) {
	if err := h.Store.DeleteGenre(r.PathValue("slug")); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Taxonomy) ListCollections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Store.Snapshot().Collections)
}

func (h *Taxonomy) UpsertCollection(w http.ResponseWriter, r *http.Request) {
	var c manifest.Collection
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if c.Slug == "" || c.Name == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("slug and name required"))
		return
	}
	if err := h.Store.UpsertCollection(c); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (h *Taxonomy) DeleteCollection(w http.ResponseWriter, r *http.Request) {
	if err := h.Store.DeleteCollection(r.PathValue("slug")); err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
