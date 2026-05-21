// Package generate renders the static site from a photos.json manifest.
package generate

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"

	"github.com/masnun/photos/admin/manifest"
)

var md = goldmark.New()

func renderMarkdown(src string) template.HTML {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(buf.String())
}

func renderMarkdownInline(src string) template.HTML {
	out := string(renderMarkdown(src))
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "<p>")
	out = strings.TrimSuffix(out, "</p>")
	return template.HTML(out)
}

var templateFuncs = template.FuncMap{
	"markdown":       renderMarkdown,
	"markdownInline": renderMarkdownInline,
}

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
	Photo           manifest.Photo
	TakenFormatted  string
	GenreLinks      []manifest.Genre
	CollectionLinks []manifest.Collection
	Prev            *manifest.Photo
	Next            *manifest.Photo
	ContextKind     string // "genre" | "collection" | "calendar"
	ContextSlug     string
	ContextName     string
	ContextURL      string
	PrevURL         string
	NextURL         string
}

type renderCtx struct {
	tmpls       map[string]*template.Template
	m           *manifest.Manifest
	outDir      string
	genreBySlug map[string]manifest.Genre
	colBySlug   map[string]manifest.Collection
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

	rc := &renderCtx{
		tmpls:       tmpls,
		m:           m,
		outDir:      opts.OutDir,
		genreBySlug: map[string]manifest.Genre{},
		colBySlug:   map[string]manifest.Collection{},
	}
	for _, g := range m.Genres {
		rc.genreBySlug[g.Slug] = g
	}
	for _, c := range m.Collections {
		rc.colBySlug[c.Slug] = c
	}

	if err := renderIndex(rc); err != nil {
		return err
	}
	if err := renderGenres(rc); err != nil {
		return err
	}
	if err := renderCollections(rc); err != nil {
		return err
	}
	if err := renderPhotos(rc); err != nil {
		return err
	}
	if err := renderCalendar(rc); err != nil {
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
	sortByTaken(out)
	return out
}

func renderCalendar(rc *renderCtx) error {
	months := buildMonths(rc.m.Photos)
	for _, mv := range months {
		photos := filterByMonth(rc.m.Photos, mv.Slug)
		mvCopy := mv
		data := listData{Month: &mvCopy, Cover: mv.Cover, Photos: photos}
		path := filepath.Join(rc.outDir, "calendar", mv.Slug, "index.html")
		if err := writeTemplate(rc.tmpls["calendar.html"], path, data); err != nil {
			return fmt.Errorf("calendar %s: %w", mv.Slug, err)
		}
		if err := renderContextPhotos(rc, photos, "calendar", mv.Slug, mv.Label, fmt.Sprintf("/calendar/%s/", mv.Slug)); err != nil {
			return err
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
		t, err := template.New("base.html").Funcs(templateFuncs).ParseFS(templatesFS, "templates/base.html", "templates/"+n)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		out[n] = t
	}
	return out, nil
}

func renderIndex(rc *renderCtx) error {
	data := indexData{
		Genres:      make([]genreView, 0, len(rc.m.Genres)),
		Collections: make([]collectionView, 0, len(rc.m.Collections)),
	}
	for _, g := range rc.m.Genres {
		photos := filterByGenre(rc.m.Photos, g.Slug)
		thumbs := make([]string, 0, len(photos))
		for _, p := range photos {
			thumbs = append(thumbs, p.URLs.Thumb)
		}
		buf, _ := json.Marshal(thumbs)
		data.Genres = append(data.Genres, genreView{Genre: g, Cover: findGenreCover(rc.m.Photos, g), ThumbsJSON: string(buf)})
	}
	for _, c := range rc.m.Collections {
		data.Collections = append(data.Collections, collectionView{Collection: c, Cover: findCollectionCover(rc.m.Photos, c)})
	}
	data.Months = buildMonths(rc.m.Photos)
	return writeTemplate(rc.tmpls["index.html"], filepath.Join(rc.outDir, "index.html"), data)
}

func renderGenres(rc *renderCtx) error {
	for _, g := range rc.m.Genres {
		photos := filterByGenre(rc.m.Photos, g.Slug)
		gCopy := g
		data := listData{Genre: &gCopy, Cover: findGenreCover(rc.m.Photos, g), Photos: photos}
		path := filepath.Join(rc.outDir, "genre", g.Slug, "index.html")
		if err := writeTemplate(rc.tmpls["genre.html"], path, data); err != nil {
			return fmt.Errorf("genre %s: %w", g.Slug, err)
		}
		if err := renderContextPhotos(rc, photos, "genre", g.Slug, g.Name, fmt.Sprintf("/genre/%s/", g.Slug)); err != nil {
			return err
		}
	}
	return nil
}

func renderCollections(rc *renderCtx) error {
	for _, c := range rc.m.Collections {
		photos := filterByCollection(rc.m.Photos, c.Slug)
		cCopy := c
		data := listData{Collection: &cCopy, Cover: findCollectionCover(rc.m.Photos, c), Photos: photos}
		path := filepath.Join(rc.outDir, "collection", c.Slug, "index.html")
		if err := writeTemplate(rc.tmpls["collection.html"], path, data); err != nil {
			return fmt.Errorf("collection %s: %w", c.Slug, err)
		}
		if err := renderContextPhotos(rc, photos, "collection", c.Slug, c.Name, fmt.Sprintf("/collection/%s/", c.Slug)); err != nil {
			return err
		}
	}
	return nil
}

func renderPhotos(rc *renderCtx) error {
	for _, p := range rc.m.Photos {
		data := buildPhotoData(rc, p)
		path := filepath.Join(rc.outDir, "photo", p.ID, "index.html")
		if err := writeTemplate(rc.tmpls["photo.html"], path, data); err != nil {
			return fmt.Errorf("photo %s: %w", p.ID, err)
		}
	}
	return nil
}

func buildPhotoData(rc *renderCtx, p manifest.Photo) photoData {
	data := photoData{Photo: p}
	if p.TakenAt != nil {
		data.TakenFormatted = p.TakenAt.Format("January 2, 2006")
	}
	for _, slug := range p.Genres {
		if g, ok := rc.genreBySlug[slug]; ok {
			data.GenreLinks = append(data.GenreLinks, g)
		}
	}
	for _, slug := range p.Collections {
		if c, ok := rc.colBySlug[slug]; ok {
			data.CollectionLinks = append(data.CollectionLinks, c)
		}
	}
	return data
}

func renderContextPhotos(rc *renderCtx, photos []manifest.Photo, kind, slug, name, ctxURL string) error {
	for i, p := range photos {
		data := buildPhotoData(rc, p)
		data.ContextKind = kind
		data.ContextSlug = slug
		data.ContextName = name
		data.ContextURL = ctxURL
		if i > 0 {
			prev := photos[i-1]
			data.Prev = &prev
			data.PrevURL = fmt.Sprintf("%sphoto/%s/", ctxURL, prev.ID)
		}
		if i < len(photos)-1 {
			next := photos[i+1]
			data.Next = &next
			data.NextURL = fmt.Sprintf("%sphoto/%s/", ctxURL, next.ID)
		}
		path := filepath.Join(rc.outDir, kind, slug, "photo", p.ID, "index.html")
		if err := writeTemplate(rc.tmpls["photo.html"], path, data); err != nil {
			return fmt.Errorf("%s/%s/photo/%s: %w", kind, slug, p.ID, err)
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
	sortByTaken(out)
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
	sortByTaken(out)
	return out
}

// sortByTaken orders photos newest-first by TakenAt with UploadedAt fallback.
func sortByTaken(out []manifest.Photo) {
	sort.SliceStable(out, func(i, j int) bool {
		ai, bi := out[i].TakenAt, out[j].TakenAt
		switch {
		case ai != nil && bi != nil:
			return ai.After(*bi)
		case ai != nil:
			return true
		case bi != nil:
			return false
		default:
			return out[i].UploadedAt.After(out[j].UploadedAt)
		}
	})
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
