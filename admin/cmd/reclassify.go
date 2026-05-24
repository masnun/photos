package cmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/masnun/photos/admin/classifier"
	"github.com/masnun/photos/admin/images"
	"github.com/masnun/photos/admin/manifest"
)

func Reclassify(args []string) {
	fs := flag.NewFlagSet("reclassify", flag.ExitOnError)
	manifestPath := fs.String("manifest", "../site/data/photos.json", "path to photos.json")
	concurrency := fs.Int("concurrency", 4, "parallel API calls")
	model := fs.String("model", "claude-haiku-4-5-20251001", "Anthropic model id")
	dryRun := fs.Bool("dry-run", false, "log proposed changes without saving")
	only := fs.String("only", "", "process only this photo id (debug)")
	fs.Parse(args)

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("missing ANTHROPIC_API_KEY")
	}

	store, err := manifest.New(*manifestPath)
	if err != nil {
		log.Fatalf("open manifest: %v", err)
	}
	snap := store.Snapshot()

	allowed := make([]string, 0, len(snap.Genres))
	for _, g := range snap.Genres {
		allowed = append(allowed, g.Slug)
	}
	if len(allowed) == 0 {
		log.Fatal("no genres in manifest; nothing to lock against")
	}
	log.Printf("locked genre list: %s", strings.Join(allowed, ", "))

	targets := snap.Photos
	if *only != "" {
		filtered := targets[:0]
		for _, p := range snap.Photos {
			if p.ID == *only {
				filtered = append(filtered, p)
				break
			}
		}
		if len(filtered) == 0 {
			log.Fatalf("photo %s not found", *only)
		}
		targets = filtered
	}
	total := len(targets)
	log.Printf("reclassifying %d photos (concurrency=%d, model=%s, dry-run=%v)",
		total, *concurrency, *model, *dryRun)

	client := classifier.New(apiKey, *model)
	httpClient := &http.Client{Timeout: 90 * time.Second}

	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup
	var doneCount, failCount int64

	for _, p := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(ph manifest.Photo) {
			defer wg.Done()
			defer func() { <-sem }()

			body, err := fetch(httpClient, ph.URLs.Full)
			if err != nil {
				log.Printf("FAIL %s fetch: %v", ph.ID, err)
				atomic.AddInt64(&failCount, 1)
				return
			}
			resized, err := images.ResizeForVision(body, 1568)
			if err != nil {
				log.Printf("FAIL %s resize: %v", ph.ID, err)
				atomic.AddInt64(&failCount, 1)
				return
			}
			res, err := client.ClassifyLocked(context.Background(), ph.Filename, resized, allowed)
			if err != nil {
				log.Printf("FAIL %s classify: %v", ph.ID, err)
				atomic.AddInt64(&failCount, 1)
				return
			}
			if len(res.Genres) == 0 {
				log.Printf("WARN %s no genres returned (model picked nothing from allowed list)", ph.ID)
			}

			n := atomic.AddInt64(&doneCount, 1)
			log.Printf("[%d/%d] %s %s -> [%s] (was [%s])",
				n, total, ph.ID, ph.Filename,
				strings.Join(res.Genres, ","), strings.Join(ph.Genres, ","))

			if *dryRun {
				return
			}
			if err := store.UpdatePhoto(ph.ID, func(p *manifest.Photo) {
				p.Genres = res.Genres
			}); err != nil {
				log.Printf("FAIL %s save: %v", ph.ID, err)
				atomic.AddInt64(&failCount, 1)
			}
		}(p)
	}
	wg.Wait()

	log.Printf("done: %d updated, %d failed", doneCount, failCount)
	if *dryRun {
		fmt.Println("(dry-run: manifest unchanged)")
	}
}

func fetch(c *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
