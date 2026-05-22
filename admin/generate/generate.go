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

const (
	siteBaseURL     = "https://masnun.photos"
	siteName        = "masnun.photos"
	siteLocale      = "en_US"
	siteDescription = "Photography by Abu Ashraf Masnun — galleries by genre, collection, and date."
)

// seoMeta carries the per-page fields the base template needs for canonical,
// OpenGraph, and meta-description tags. Every page data struct embeds one.
type seoMeta struct {
	BaseURL     string
	SiteName    string
	Locale      string
	Path        string // absolute site path, e.g. "/genre/street/"
	Description string
	ImageURL    string // og:image; empty omits the tag
}

func newSEO(path, description, imageURL string) seoMeta {
	return seoMeta{
		BaseURL:     siteBaseURL,
		SiteName:    siteName,
		Locale:      siteLocale,
		Path:        path,
		Description: description,
		ImageURL:    imageURL,
	}
}

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
	SEO         seoMeta
	Featured    []manifest.Photo
	Genres      []genreView
	Collections []collectionView
	Months      []monthView
}

type monthGroup struct {
	Slug   string
	Label  string
	Photos []manifest.Photo
}

type listData struct {
	SEO         seoMeta
	Genre       *manifest.Genre
	Collection  *manifest.Collection
	Month       *monthView
	Cover       *manifest.Photo
	Photos      []manifest.Photo
	MonthGroups []monthGroup
}

type photoData struct {
	SEO             seoMeta
	JSONLD          template.JS
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
	if err := renderTaxonomyIndexes(rc); err != nil {
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
	if err := renderSitemap(rc); err != nil {
		return fmt.Errorf("sitemap: %w", err)
	}
	if err := renderRobots(rc); err != nil {
		return fmt.Errorf("robots: %w", err)
	}
	if err := renderWebmanifest(rc); err != nil {
		return fmt.Errorf("webmanifest: %w", err)
	}
	return nil
}

func renderTaxonomyIndexes(rc *renderCtx) error {
	data := buildIndexData(rc)
	data.SEO = newSEO("/genre/", "Browse photographs by genre.", "")
	if err := writeTemplate(rc.tmpls["genres-index.html"], filepath.Join(rc.outDir, "genre", "index.html"), data); err != nil {
		return fmt.Errorf("genres index: %w", err)
	}
	data.SEO = newSEO("/collection/", "Browse curated photo collections.", "")
	if err := writeTemplate(rc.tmpls["collections-index.html"], filepath.Join(rc.outDir, "collection", "index.html"), data); err != nil {
		return fmt.Errorf("collections index: %w", err)
	}
	data.SEO = newSEO("/calendar/", "Browse photographs by month.", "")
	if err := writeTemplate(rc.tmpls["calendar-index.html"], filepath.Join(rc.outDir, "calendar", "index.html"), data); err != nil {
		return fmt.Errorf("calendar index: %w", err)
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
		data.SEO = newSEO(fmt.Sprintf("/calendar/%s/", mv.Slug), fmt.Sprintf("Photographs from %s.", mv.Label), coverURL(mv.Cover))
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
	names := []string{
		"index.html",
		"genre.html",
		"collection.html",
		"photo.html",
		"calendar.html",
		"genres-index.html",
		"collections-index.html",
		"calendar-index.html",
	}
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
	data := buildIndexData(rc)
	var heroImg string
	if len(data.Featured) > 0 {
		heroImg = data.Featured[0].URLs.Web
	}
	data.SEO = newSEO("/", siteDescription, heroImg)
	return writeTemplate(rc.tmpls["index.html"], filepath.Join(rc.outDir, "index.html"), data)
}

func buildIndexData(rc *renderCtx) indexData {
	data := indexData{
		Genres:      make([]genreView, 0, len(rc.m.Genres)),
		Collections: make([]collectionView, 0, len(rc.m.Collections)),
	}
	for _, g := range rc.m.Genres {
		photos := filterByGenre(rc.m.Photos, g.Slug, g.Order)
		thumbs := make([]string, 0, len(photos))
		for _, p := range photos {
			thumbs = append(thumbs, p.URLs.Thumb)
		}
		buf, _ := json.Marshal(thumbs)
		data.Genres = append(data.Genres, genreView{Genre: g, Cover: findGenreCover(rc.m.Photos, g), ThumbsJSON: string(buf)})
	}
	for _, c := range rc.m.Collections {
		if c.Slug == manifest.FeaturedSlug {
			continue
		}
		data.Collections = append(data.Collections, collectionView{Collection: c, Cover: findCollectionCover(rc.m.Photos, c)})
	}
	data.Featured = filterByCollection(rc.m.Photos, manifest.FeaturedSlug, rc.colBySlug[manifest.FeaturedSlug].Order)
	data.Months = buildMonths(rc.m.Photos)
	return data
}

func renderGenres(rc *renderCtx) error {
	for _, g := range rc.m.Genres {
		photos := filterByGenre(rc.m.Photos, g.Slug, g.Order)
		gCopy := g
		cover := findGenreCover(rc.m.Photos, g)
		data := listData{Genre: &gCopy, Cover: cover, Photos: photos, MonthGroups: buildMonthGroups(photos)}
		desc := g.Description
		if strings.TrimSpace(desc) == "" {
			desc = fmt.Sprintf("Photographs in the %s genre.", g.Name)
		}
		data.SEO = newSEO(fmt.Sprintf("/genre/%s/", g.Slug), desc, coverURL(cover))
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
		photos := filterByCollection(rc.m.Photos, c.Slug, c.Order)
		cCopy := c
		cover := findCollectionCover(rc.m.Photos, c)
		data := listData{Collection: &cCopy, Cover: cover, Photos: photos, MonthGroups: buildMonthGroups(photos)}
		desc := c.Description
		if strings.TrimSpace(desc) == "" {
			desc = fmt.Sprintf("Photographs in the %s collection.", c.Name)
		}
		data.SEO = newSEO(fmt.Sprintf("/collection/%s/", c.Slug), desc, coverURL(cover))
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
	desc := p.Caption
	if strings.TrimSpace(desc) == "" {
		desc = siteDescription
	}
	data.SEO = newSEO(fmt.Sprintf("/photo/%s/", p.ID), desc, p.URLs.Web)
	data.JSONLD = photoJSONLD(p)
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

func filterByGenre(photos []manifest.Photo, slug string, order []string) []manifest.Photo {
	out := make([]manifest.Photo, 0)
	for _, p := range photos {
		for _, s := range p.Genres {
			if s == slug {
				out = append(out, p)
				break
			}
		}
	}
	return applyOrder(out, order)
}

func filterByCollection(photos []manifest.Photo, slug string, order []string) []manifest.Photo {
	out := make([]manifest.Photo, 0)
	for _, p := range photos {
		for _, s := range p.Collections {
			if s == slug {
				out = append(out, p)
				break
			}
		}
	}
	return applyOrder(out, order)
}

// applyOrder sorts photos by the operator-defined order (a slice of photo IDs).
// Photos absent from order (e.g. added after the last reorder) are appended
// after the ordered ones, oldest-first by TakenAt. No order falls back to
// chronological.
func applyOrder(photos []manifest.Photo, order []string) []manifest.Photo {
	if len(order) == 0 {
		sortByTaken(photos)
		return photos
	}
	idx := make(map[string]int, len(order))
	for i, id := range order {
		idx[id] = i
	}
	known := make([]manifest.Photo, 0, len(photos))
	unknown := make([]manifest.Photo, 0)
	for _, p := range photos {
		if _, ok := idx[p.ID]; ok {
			known = append(known, p)
		} else {
			unknown = append(unknown, p)
		}
	}
	sort.SliceStable(known, func(i, j int) bool {
		return idx[known[i].ID] < idx[known[j].ID]
	})
	sortByTaken(unknown)
	return append(known, unknown...)
}

// buildMonthGroups buckets photos into month sections (newest month first,
// undated last), each section's photos oldest-first by TakenAt. Powers the
// "by month" toggle on genre and collection pages.
func buildMonthGroups(photos []manifest.Photo) []monthGroup {
	groups := map[string][]manifest.Photo{}
	for _, p := range photos {
		key := monthKey(p)
		groups[key] = append(groups[key], p)
	}
	out := make([]monthGroup, 0, len(groups))
	for key, ps := range groups {
		mg := monthGroup{Slug: key, Photos: ps}
		if key == "undated" {
			mg.Label = "Undated"
		} else if t, err := time.Parse("2006-01", key); err == nil {
			mg.Label = t.Format("January 2006")
		} else {
			mg.Label = key
		}
		sortByTaken(mg.Photos)
		out = append(out, mg)
	}
	sort.Slice(out, func(i, j int) bool {
		ui, uj := out[i].Slug == "undated", out[j].Slug == "undated"
		if ui != uj {
			return !ui
		}
		return out[i].Slug > out[j].Slug
	})
	return out
}

// sortByTaken orders photos oldest-first by TakenAt with UploadedAt fallback.
func sortByTaken(out []manifest.Photo) {
	sort.SliceStable(out, func(i, j int) bool {
		ai, bi := out[i].TakenAt, out[j].TakenAt
		switch {
		case ai != nil && bi != nil:
			return ai.Before(*bi)
		case ai != nil:
			return true
		case bi != nil:
			return false
		default:
			return out[i].UploadedAt.Before(out[j].UploadedAt)
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

func coverURL(p *manifest.Photo) string {
	if p == nil {
		return ""
	}
	return p.URLs.Web
}

// photoJSONLD builds a schema.org ImageObject blob for a photo detail page.
func photoJSONLD(p manifest.Photo) template.JS {
	ld := map[string]any{
		"@context":     "https://schema.org",
		"@type":        "ImageObject",
		"contentUrl":   p.URLs.Full,
		"thumbnailUrl": p.URLs.Thumb,
		"url":          fmt.Sprintf("%s/photo/%s/", siteBaseURL, p.ID),
		"creator": map[string]any{
			"@type": "Person",
			"name":  "Abu Ashraf Masnun",
		},
		"copyrightHolder": map[string]any{
			"@type": "Person",
			"name":  "Abu Ashraf Masnun",
		},
	}
	if p.Caption != "" {
		ld["name"] = p.Caption
		ld["description"] = p.Caption
		ld["caption"] = p.Caption
	}
	if p.Width > 0 {
		ld["width"] = p.Width
	}
	if p.Height > 0 {
		ld["height"] = p.Height
	}
	if p.TakenAt != nil && !p.TakenAt.IsZero() {
		ld["dateCreated"] = p.TakenAt.Format(time.RFC3339)
	}
	buf, err := json.Marshal(ld)
	if err != nil {
		return ""
	}
	return template.JS(buf)
}

// renderSitemap writes sitemap.xml listing every canonical URL. Context photo
// pages (/<kind>/<slug>/photo/<id>/) are intentionally omitted — they canonical
// back to /photo/<id>/.
func renderSitemap(rc *renderCtx) error {
	var urls []string
	urls = append(urls, "/", "/genre/", "/collection/", "/calendar/")
	for _, g := range rc.m.Genres {
		urls = append(urls, fmt.Sprintf("/genre/%s/", g.Slug))
	}
	for _, c := range rc.m.Collections {
		urls = append(urls, fmt.Sprintf("/collection/%s/", c.Slug))
	}
	for _, mv := range buildMonths(rc.m.Photos) {
		urls = append(urls, fmt.Sprintf("/calendar/%s/", mv.Slug))
	}
	for _, p := range rc.m.Photos {
		urls = append(urls, fmt.Sprintf("/photo/%s/", p.ID))
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, u := range urls {
		b.WriteString("  <url><loc>")
		b.WriteString(template.HTMLEscapeString(siteBaseURL + u))
		b.WriteString("</loc></url>\n")
	}
	b.WriteString("</urlset>\n")
	return writeFile(filepath.Join(rc.outDir, "sitemap.xml"), b.String())
}

func renderRobots(rc *renderCtx) error {
	body := fmt.Sprintf("User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml\n", siteBaseURL)
	return writeFile(filepath.Join(rc.outDir, "robots.txt"), body)
}

func renderWebmanifest(rc *renderCtx) error {
	m := map[string]any{
		"name":             siteName,
		"short_name":       siteName,
		"description":      siteDescription,
		"start_url":        "/",
		"display":          "standalone",
		"background_color": "#111111",
		"theme_color":      "#111111",
		"icons": []map[string]any{
			{"src": "/assets/img/favicon-192x192.png", "sizes": "192x192", "type": "image/png"},
			{"src": "/assets/img/favicon-512x512.png", "sizes": "512x512", "type": "image/png"},
		},
	}
	buf, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(rc.outDir, "site.webmanifest"), string(buf)+"\n")
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
