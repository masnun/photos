package review

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/masnun/photos/admin/images"
)

type Row struct {
	Idx        int      `json:"idx"`
	Path       string   `json:"path"`
	Hash       string   `json:"hash"`
	Genres     []string `json:"genres"`
	Collection string   `json:"collection"`
}

type store struct {
	tsvPath string
	dir     string
	dirAbs  string
	mu      sync.RWMutex
	rows    []Row
}

func New(tsvPath, photosDir string) (http.Handler, error) {
	dirAbs, err := filepath.Abs(photosDir)
	if err != nil {
		return nil, err
	}
	s := &store{tsvPath: tsvPath, dir: photosDir, dirAbs: dirAbs}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s.handler(), nil
}

func (s *store) load() error {
	f, err := os.Open(s.tsvPath)
	if err != nil {
		return err
	}
	defer f.Close()
	rows := []Row{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		line := sc.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		for len(parts) < 4 {
			parts = append(parts, "")
		}
		genres := []string{}
		for _, g := range strings.Split(parts[2], ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				genres = append(genres, g)
			}
		}
		// collection is the last column; reading it from the end keeps stale
		// 5-column (caption-bearing) TSVs parsing correctly.
		rows = append(rows, Row{
			Idx:        len(rows),
			Path:       strings.TrimSpace(parts[0]),
			Hash:       strings.TrimSpace(parts[1]),
			Genres:     genres,
			Collection: strings.TrimSpace(parts[len(parts)-1]),
		})
	}
	s.rows = rows
	return sc.Err()
}

func (s *store) save() error {
	tmp := s.tsvPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, "path\tsha256\tgenres\tcollection"); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	for _, r := range s.rows {
		_, err := fmt.Fprintf(f, "%s\t%s\t%s\t%s\n",
			r.Path, r.Hash,
			strings.Join(r.Genres, ","),
			sanitize(r.Collection),
		)
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.tsvPath)
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func (s *store) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/rows", s.list)
	mux.HandleFunc("PATCH /api/rows/{idx}", s.update)
	mux.HandleFunc("DELETE /api/rows/{idx}", s.deleteRow)
	mux.HandleFunc("POST /api/reload", s.reload)
	mux.HandleFunc("GET /thumb/", s.thumb)
	mux.HandleFunc("GET /full/", s.full)
	mux.Handle("/", uiHandler())
	return mux
}

func (s *store) list(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	writeJSON(w, http.StatusOK, s.rows)
}

type patchBody struct {
	Genres     *[]string `json:"genres,omitempty"`
	Collection *string   `json:"collection,omitempty"`
}

func (s *store) update(w http.ResponseWriter, r *http.Request) {
	var body patchBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := parseIdx(s.rows, r.PathValue("idx"))
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if body.Genres != nil {
		s.rows[idx].Genres = normalize(*body.Genres)
	}
	if body.Collection != nil {
		s.rows[idx].Collection = strings.TrimSpace(*body.Collection)
	}
	if err := s.save(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.rows[idx])
}

func (s *store) deleteRow(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := parseIdx(s.rows, r.PathValue("idx"))
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.rows = append(s.rows[:idx], s.rows[idx+1:]...)
	for i := range s.rows {
		s.rows[i].Idx = i
	}
	if err := s.save(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *store) reload(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.load(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"rows": len(s.rows)})
}

func (s *store) thumb(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/thumb/")
	full, err := s.safePath(rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	out, err := images.ResizeForVision(data, 600)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Write(out)
}

func (s *store) full(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, "/full/")
	full, err := s.safePath(rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.ServeFile(w, r, full)
}

func (s *store) safePath(rel string) (string, error) {
	abs, err := filepath.Abs(filepath.Join(s.dir, rel))
	if err != nil {
		return "", err
	}
	if abs != s.dirAbs && !strings.HasPrefix(abs, s.dirAbs+string(filepath.Separator)) {
		return "", fmt.Errorf("path escape")
	}
	return abs, nil
}

func parseIdx(rows []Row, s string) (int, bool) {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n >= len(rows) {
		return 0, false
	}
	return n, true
}

func normalize(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		s = strings.ReplaceAll(s, " ", "-")
		s = strings.ReplaceAll(s, "_", "-")
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
