package cmd

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/masnun/photos/admin/images"
	"github.com/masnun/photos/admin/manifest"
	"github.com/masnun/photos/admin/storage"
)

func Ingest(args []string) {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	tsv := fs.String("tsv", "classifications.tsv", "input TSV path")
	dir := fs.String("dir", "../photos", "source directory (paths in TSV are relative to this)")
	manifestPath := fs.String("manifest", "../site/data/photos.json", "manifest path")
	concurrency := fs.Int("concurrency", 4, "parallel uploads")
	fs.Parse(args)

	cfg := r2Config()
	r2, err := storage.New(context.Background(), cfg)
	if err != nil {
		log.Fatalf("r2 init: %v", err)
	}

	store, err := manifest.New(*manifestPath)
	if err != nil {
		log.Fatalf("manifest: %v", err)
	}

	rows, err := readTSV(*tsv)
	if err != nil {
		log.Fatalf("read tsv: %v", err)
	}
	log.Printf("loaded %d rows from %s", len(rows), *tsv)

	existingHashes := map[string]bool{}
	for _, p := range store.Snapshot().Photos {
		if p.SourceHash != "" {
			existingHashes[p.SourceHash] = true
		}
	}

	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup
	var doneMu sync.Mutex
	var done, skipped int

	for i, row := range rows {
		if len(row.Genres) == 0 {
			log.Printf("skip %s: no genres in TSV", row.Path)
			skipped++
			continue
		}
		if row.Hash != "" && existingHashes[row.Hash] {
			log.Printf("skip %s: hash already in manifest", row.Path)
			skipped++
			continue
		}
		ensureTaxonomies(store, row.Genres, row.Collection)

		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, row tsvRow) {
			defer wg.Done()
			defer func() { <-sem }()

			full := filepath.Join(*dir, row.Path)
			data, err := os.ReadFile(full)
			if err != nil {
				log.Printf("read %s: %v", row.Path, err)
				return
			}

			sum := sha256.Sum256(data)
			hash := hex.EncodeToString(sum[:])
			if row.Hash != "" && row.Hash != hash {
				log.Printf("warn %s: hash drift (tsv=%s file=%s) — file changed since classify", row.Path, row.Hash, hash)
			}

			v, err := images.Process(data)
			if err != nil {
				log.Printf("process %s: %v", row.Path, err)
				return
			}

			exifData, takenAt, _ := images.ExtractEXIF(data)

			id := uuid.NewString()
			ext := strings.ToLower(filepath.Ext(row.Path))
			if ext == "" {
				ext = ".jpg"
			}

			ctx := context.Background()
			thumbURL, err := r2.Put(ctx, fmt.Sprintf("photos/%s/thumb.jpg", id), v.Thumb, "image/jpeg")
			if err != nil {
				log.Printf("put thumb %s: %v", row.Path, err)
				return
			}
			webURL, err := r2.Put(ctx, fmt.Sprintf("photos/%s/web.jpg", id), v.Web, "image/jpeg")
			if err != nil {
				log.Printf("put web %s: %v", row.Path, err)
				return
			}
			fullURL, err := r2.Put(ctx, fmt.Sprintf("photos/%s/full%s", id, ext), v.Full, mimeFor(ext))
			if err != nil {
				log.Printf("put full %s: %v", row.Path, err)
				return
			}

			collections := []string{}
			if row.Collection != "" {
				collections = []string{row.Collection}
			}

			p := manifest.Photo{
				ID:          id,
				Filename:    filepath.Base(row.Path),
				Caption:     row.Caption,
				Genres:      row.Genres,
				Collections: collections,
				TakenAt:     takenAt,
				UploadedAt:  time.Now().UTC(),
				Width:       v.Width,
				Height:      v.Height,
				URLs:        manifest.URLs{Thumb: thumbURL, Web: webURL, Full: fullURL},
				EXIF:        exifData,
				SourceHash:  hash,
			}
			if err := store.AddPhoto(p); err != nil {
				log.Printf("save %s: %v", row.Path, err)
				return
			}
			doneMu.Lock()
			done++
			n := done
			doneMu.Unlock()
			log.Printf("[%d/%d] uploaded %s id=%s genres=%s", n, len(rows), row.Path, id, strings.Join(row.Genres, ","))
		}(i, row)
	}
	wg.Wait()
	log.Printf("done: ingested=%d skipped=%d", done, skipped)
}

type tsvRow struct {
	Path       string
	Hash       string
	Genres     []string
	Caption    string
	Collection string
}

func readTSV(path string) ([]tsvRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var rows []tsvRow
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
		for len(parts) < 5 {
			parts = append(parts, "")
		}
		genres := []string{}
		for _, g := range strings.Split(parts[2], ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				genres = append(genres, g)
			}
		}
		rows = append(rows, tsvRow{
			Path:       strings.TrimSpace(parts[0]),
			Hash:       strings.TrimSpace(parts[1]),
			Genres:     genres,
			Caption:    parts[3],
			Collection: strings.TrimSpace(parts[4]),
		})
	}
	return rows, sc.Err()
}

func ensureTaxonomies(store *manifest.Store, genres []string, collection string) {
	snap := store.Snapshot()
	have := map[string]bool{}
	for _, g := range snap.Genres {
		have[g.Slug] = true
	}
	for _, g := range genres {
		if !have[g] {
			_ = store.UpsertGenre(manifest.Genre{Slug: g, Name: titleize(g)})
			have[g] = true
		}
	}
	if collection != "" {
		haveCol := false
		for _, c := range snap.Collections {
			if c.Slug == collection {
				haveCol = true
				break
			}
		}
		if !haveCol {
			_ = store.UpsertCollection(manifest.Collection{Slug: collection, Name: titleize(collection)})
		}
	}
}

func titleize(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
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
