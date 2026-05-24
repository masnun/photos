package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/masnun/photos/admin/images"
	"github.com/masnun/photos/admin/manifest"
	"github.com/masnun/photos/admin/storage"
)

type Photos struct {
	Store *manifest.Store
	R2    *storage.R2
}

func (h *Photos) List(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Store.Snapshot().Photos)
}

func (h *Photos) Upload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(128 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("no file"))
		return
	}

	ctx := r.Context()
	out := []manifest.Photo{}
	for _, fh := range files {
		f, err := fh.Open()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}

		v, err := images.Process(data)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}

		exifData, takenAt, _ := images.ExtractEXIF(data)

		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])

		id := uuid.NewString()
		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if ext == "" {
			ext = ".jpg"
		}

		keyThumb := fmt.Sprintf("photos/%s/thumb.jpg", id)
		keyWeb := fmt.Sprintf("photos/%s/web.jpg", id)
		keyFull := fmt.Sprintf("photos/%s/full%s", id, ext)

		thumbURL, err := h.R2.Put(ctx, keyThumb, v.Thumb, "image/jpeg")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		webURL, err := h.R2.Put(ctx, keyWeb, v.Web, "image/jpeg")
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		fullURL, err := h.R2.Put(ctx, keyFull, v.Full, mimeFor(ext))
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}

		p := manifest.Photo{
			ID:          id,
			Filename:    fh.Filename,
			Genres:      []string{},
			Collections: []string{},
			TakenAt:     takenAt,
			UploadedAt:  time.Now().UTC(),
			Width:       v.Width,
			Height:      v.Height,
			URLs:        manifest.URLs{Thumb: thumbURL, Web: webURL, Full: fullURL},
			EXIF:        exifData,
			SourceHash:  hash,
		}
		if err := h.Store.AddPhoto(p); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, p)
	}
	writeJSON(w, http.StatusCreated, out)
}

type photoPatch struct {
	Genres      *[]string `json:"genres,omitempty"`
	Collections *[]string `json:"collections,omitempty"`
}

func (h *Photos) Patch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body photoPatch
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	err := h.Store.UpdatePhoto(id, func(p *manifest.Photo) {
		if body.Genres != nil {
			p.Genres = *body.Genres
		}
		if body.Collections != nil {
			p.Collections = *body.Collections
		}
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Photos) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.Store.DeletePhoto(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx := r.Context()
	for _, u := range []string{p.URLs.Thumb, p.URLs.Web, p.URLs.Full} {
		if k := h.R2.KeyFromURL(u); k != "" {
			_ = h.R2.Delete(ctx, k)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func mimeFor(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/jpeg"
	}
}
