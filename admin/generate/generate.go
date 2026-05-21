// Package generate renders the static site from a photos.json manifest.
package generate

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/masnun/photos/admin/manifest"
)

//go:embed templates/*.html
var templatesFS embed.FS

type Options struct {
	ManifestPath string
	OutDir       string
}

// genreView pairs a Genre with a resolved cover photo for tile rendering.
type genreView struct {
	manifest.Genre
	Cover      *manifest.Photo
	ThumbsJSON string
}

type collectionView struct {
	manifest.Collection
	Cover *manifest.Photo
}

type monthView struct {
	Slug    string // "2024-08" or "undated"
	Label   string // "August 2024" or "Undated"
	Count   int
	Cover   *manifest.Photo
	Undated bool
}

type indexData struct {
	Genres      []genreView
	Collections []collectionView
	Months      []monthView
}

type listData struct {
	Genre      *manifest.Genre
	Collection *manifest.Collection
	Month      *monthView
	Cover      *manifest.Photo
	Photos     []manifest.Photo
}

type photoData struct {
	Photo            manifest.Photo
	TakenFormatted   string
	GenreLinks       []manifest.Genre
	CollectionLinks  []manifest.Collection
}

func Run(opts Options) error {
	m, err := loadManifest(opts.ManifestPath)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	sort.SliceStable(m.Genres, func(i, j int) bool {
		return strings.ToLower(m.Genres[i].Name) < strings.ToLower(m.Genres[j].Name)
	})

	tmpls, err := parseTemplates()
	if err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}

	if err := renderIndex(tmpls, m, opts.OutDir); err != nil {
		return err
	}
	if err := renderGenres(tmpls, m, opts.OutDir); err != nil {
		return err
	}
	if err := renderCollections(tmpls, m, opts.OutDir); err != nil {
		return err
	}
	if err := renderPhotos(tmpls, m, opts.OutDir); err != nil {
		return err
	}
	if err := renderCalendar(tmpls, m, opts.OutDir); err != nil {
		return err
	}
	return nil
}

func buildMonths(photos []manifest.Photo) []monthView {
	groups := map[string][]manifest.Photo{}
	for _, p := range photos {
		key := monthKey(p)
		groups[key] = append(groups[key], p)
	}
	out := make([]monthView, 0, len(groups))
	for key, ps := range groups {
		mv := monthView{Slug: key, Count: len(ps)}
		if key == "undated" {
			mv.Label = "Undated"
			mv.Undated = true
		} else {
			if t, err := time.Parse("2006-01", key); err == nil {
				mv.Label = t.Format("January 2006")
			} else {
				mv.Label = key
			}
		}
		if len(ps) > 0 {
			p := ps[0]
			mv.Cover = &p
		}
		out = append(out, mv)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Undated != out[j].Undated {
			return !out[i].Undated
		}
		return out[i].Slug > out[j].Slug
	})
	return out
}

func monthKey(p manifest.Photo) string {
	if p.TakenAt != nil && !p.TakenAt.IsZero() {
		return p.TakenAt.Format("2006-01")
	}
	return "undated"
}

func filterByMonth(photos []manifest.Photo, slug string) []manifest.Photo {
	out := make([]manifest.Photo, 0)
	for _, p := range photos {
		if monthKey(p) == slug {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ai, bi := out[i].TakenAt, out[j].TakenAt
		if ai != nil && bi != nil {
			return ai.After(*bi)
		}
		return out[i].UploadedAt.After(out[j].UploadedAt)
	})
	return out
}

func renderCalendar(tmpls map[string]*template.Template, m *manifest.Manifest, outDir string) error {
	months := buildMonths(m.Photos)
	for _, mv := range months {
		photos := filterByMonth(m.Photos, mv.Slug)
		mvCopy := mv
		data := listData{Month: &mvCopy, Cover: mv.Cover, Photos: photos}
		path := filepath.Join(outDir, "calendar", mv.Slug, "index.html")
		if err := writeTemplate(tmpls["calendar.html"], path, data); err != nil {
			return fmt.Errorf("calendar %s: %w", mv.Slug, err)
		}
	}
	return nil
}

func loadManifest(path string) (*manifest.Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var m manifest.Manifest
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

func parseTemplates() (map[string]*template.Template, error) {
	names := []string{"index.html", "genre.html", "collection.html", "photo.html", "calendar.html"}
	out := make(map[string]*template.Template, len(names))
	for _, n := range names {
		t, err := template.ParseFS(templatesFS, "templates/base.html", "templates/"+n)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out[n] = t
	}
	return out, nil
}

func renderIndex(tmpls map[string]*template.Template, m *manifest.Manifest, outDir string) error {
	data := indexData{
		Genres:      make([]genreView, 0, len(m.Genres)),
		Collections: make([]collectionView, 0, len(m.Collections)),
	}
	for _, g := range m.Genres {
		photos := filterByGenre(m.Photos, g.Slug)
		thumbs := make([]string, 0, len(photos))
		for _, p := range photos {
			thumbs = append(thumbs, p.URLs.Thumb)
		}
		buf, _ := json.Marshal(thumbs)
		data.Genres = append(data.Genres, genreView{Genre: g, Cover: findGenreCover(m.Photos, g), ThumbsJSON: string(buf)})
	}
	for _, c := range m.Collections {
		data.Collections = append(data.Collections, collectionView{Collection: c, Cover: findCollectionCover(m.Photos, c)})
	}
	data.Months = buildMonths(m.Photos)
	return writeTemplate(tmpls["index.html"], filepath.Join(outDir, "index.html"), data)
}

func renderGenres(tmpls map[string]*template.Template, m *manifest.Manifest, outDir string) error {
	for _, g := range m.Genres {
		photos := filterByGenre(m.Photos, g.Slug)
		data := listData{Genre: &g, Cover: findGenreCover(m.Photos, g), Photos: photos}
		path := filepath.Join(outDir, "genre", g.Slug, "index.html")
		if err := writeTemplate(tmpls["genre.html"], path, data); err != nil {
			return fmt.Errorf("genre %s: %w", g.Slug, err)
		}
	}
	return nil
}

func renderCollections(tmpls map[string]*template.Template, m *manifest.Manifest, outDir string) error {
	for _, c := range m.Collections {
		photos := filterByCollection(m.Photos, c.Slug)
		data := listData{Collection: &c, Cover: findCollectionCover(m.Photos, c), Photos: photos}
		path := filepath.Join(outDir, "collection", c.Slug, "index.html")
		if err := writeTemplate(tmpls["collection.html"], path, data); err != nil {
			return fmt.Errorf("collection %s: %w", c.Slug, err)
		}
	}
	return nil
}

func renderPhotos(tmpls map[string]*template.Template, m *manifest.Manifest, outDir string) error {
	genreBySlug := map[string]manifest.Genre{}
	for _, g := range m.Genres {
		genreBySlug[g.Slug] = g
	}
	colBySlug := map[string]manifest.Collection{}
	for _, c := range m.Collections {
		colBySlug[c.Slug] = c
	}
	for _, p := range m.Photos {
		data := photoData{Photo: p}
		if p.TakenAt != nil {
			data.TakenFormatted = p.TakenAt.Format("January 2, 2006")
		}
		for _, slug := range p.Genres {
			if g, ok := genreBySlug[slug]; ok {
				data.GenreLinks = append(data.GenreLinks, g)
			}
		}
		for _, slug := range p.Collections {
			if c, ok := colBySlug[slug]; ok {
				data.CollectionLinks = append(data.CollectionLinks, c)
			}
		}
		path := filepath.Join(outDir, "photo", p.ID, "index.html")
		if err := writeTemplate(tmpls["photo.html"], path, data); err != nil {
			return fmt.Errorf("photo %s: %w", p.ID, err)
		}
	}
	return nil
}

func writeTemplate(t *template.Template, path string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := t.ExecuteTemplate(f, "base", data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func filterByGenre(photos []manifest.Photo, slug string) []manifest.Photo {
	out := make([]manifest.Photo, 0)
	for _, p := range photos {
		for _, s := range p.Genres {
			if s == slug {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func filterByCollection(photos []manifest.Photo, slug string) []manifest.Photo {
	out := make([]manifest.Photo, 0)
	for _, p := range photos {
		for _, s := range p.Collections {
			if s == slug {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func findGenreCover(photos []manifest.Photo, g manifest.Genre) *manifest.Photo {
	if g.Cover != "" {
		for i := range photos {
			if photos[i].ID == g.Cover {
				return &photos[i]
			}
		}
	}
	for i := range photos {
		for _, s := range photos[i].Genres {
			if s == g.Slug {
				return &photos[i]
			}
		}
	}
	return nil
}

func findCollectionCover(photos []manifest.Photo, c manifest.Collection) *manifest.Photo {
	if c.Cover != "" {
		for i := range photos {
			if photos[i].ID == c.Cover {
				return &photos[i]
			}
		}
	}
	for i := range photos {
		for _, s := range photos[i].Collections {
			if s == c.Slug {
				return &photos[i]
			}
		}
	}
	return nil
}
