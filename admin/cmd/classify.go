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

	"github.com/masnun/photos/admin/classifier"
	"github.com/masnun/photos/admin/images"
)

func Classify(args []string) {
	fs := flag.NewFlagSet("classify", flag.ExitOnError)
	dir := fs.String("dir", "../photos", "source directory of original photos")
	out := fs.String("out", "classifications.tsv", "output TSV path (appended; resumable)")
	concurrency := fs.Int("concurrency", 4, "parallel API calls")
	model := fs.String("model", "claude-haiku-4-5-20251001", "Anthropic model id")
	fs.Parse(args)

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("missing ANTHROPIC_API_KEY")
	}

	paths, err := walkPhotos(*dir)
	if err != nil {
		log.Fatalf("walk: %v", err)
	}
	log.Printf("found %d photos under %s", len(paths), *dir)

	existing, err := loadDoneSet(*out)
	if err != nil {
		log.Fatalf("read existing tsv: %v", err)
	}
	if len(existing) > 0 {
		log.Printf("resuming: %d rows already in %s", len(existing), *out)
	}

	_, statErr := os.Stat(*out)
	newFile := os.IsNotExist(statErr)

	f, err := os.OpenFile(*out, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		log.Fatalf("open out: %v", err)
	}
	defer f.Close()

	if newFile {
		fmt.Fprintln(f, "path\tsha256\tgenres\tcaption\tcollection")
	}

	var writeMu sync.Mutex
	writeRow := func(row string) {
		writeMu.Lock()
		defer writeMu.Unlock()
		fmt.Fprintln(f, row)
	}

	client := classifier.New(apiKey, *model)
	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup
	var doneMu sync.Mutex
	var done int

	for _, p := range paths {
		rel, _ := filepath.Rel(*dir, p)
		if existing[rel] {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(path, rel string) {
			defer wg.Done()
			defer func() { <-sem }()

			data, err := os.ReadFile(path)
			if err != nil {
				log.Printf("read %s: %v", rel, err)
				return
			}
			sum := sha256.Sum256(data)
			hash := hex.EncodeToString(sum[:])

			resized, err := images.ResizeForVision(data, 1568)
			if err != nil {
				log.Printf("resize %s: %v", rel, err)
				return
			}
			result, err := client.Classify(context.Background(), filepath.Base(path), resized)
			if err != nil {
				log.Printf("classify %s: %v", rel, err)
				return
			}
			row := strings.Join([]string{
				rel,
				hash,
				strings.Join(result.Genres, ","),
				sanitizeCell(result.Caption),
				sanitizeCell(result.CollectionHint),
			}, "\t")
			writeRow(row)

			doneMu.Lock()
			done++
			n := done
			doneMu.Unlock()
			log.Printf("[%d/%d] %s -> [%s] %s", n, len(paths), rel,
				strings.Join(result.Genres, ","), truncateStr(result.Caption, 60))
		}(p, rel)
	}
	wg.Wait()
	log.Printf("done: wrote %d new rows to %s", done, *out)
}

func walkPhotos(dir string) ([]string, error) {
	var paths []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && p != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}
		switch strings.ToLower(filepath.Ext(p)) {
		case ".jpg", ".jpeg", ".png", ".webp":
			paths = append(paths, p)
		}
		return nil
	})
	return paths, err
}

func loadDoneSet(path string) (map[string]bool, error) {
	out := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		line := sc.Text()
		i := strings.IndexByte(line, '\t')
		if i < 0 {
			continue
		}
		out[line[:i]] = true
	}
	return out, sc.Err()
}

func sanitizeCell(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
