package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/masnun/photos/admin/images"
	"github.com/masnun/photos/admin/manifest"
)

// BackfillGPS walks a local photos directory, hashes each image, and writes
// Lat/Lon (and missing EXIF fields) into matching manifest entries.
// Existing Lat/Lon are not overwritten unless -force is set.
func BackfillGPS(args []string) {
	flags := flag.NewFlagSet("backfill-gps", flag.ExitOnError)
	dir := flags.String("dir", "../photos", "source directory to scan")
	manifestPath := flags.String("manifest", "../site/data/photos.json", "manifest path")
	force := flags.Bool("force", false, "overwrite existing Lat/Lon")
	flags.Parse(args)

	store, err := manifest.New(*manifestPath)
	if err != nil {
		log.Fatalf("manifest: %v", err)
	}

	byHash := map[string]string{} // hash -> photo id
	for _, p := range store.Snapshot().Photos {
		if p.SourceHash == "" {
			continue
		}
		if !*force && p.Lat != nil && p.Lon != nil {
			continue
		}
		byHash[p.SourceHash] = p.ID
	}
	log.Printf("targets: %d photo(s) missing GPS", len(byHash))

	var scanned, matched, updated, missingGPS int
	walkErr := filepath.WalkDir(*dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !isImage(path) {
			return nil
		}
		scanned++
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("read %s: %v", path, err)
			return nil
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		id, ok := byHash[hash]
		if !ok {
			return nil
		}
		matched++
		_, _, lat, lon, _ := images.ExtractEXIF(data)
		if lat == nil || lon == nil {
			missingGPS++
			log.Printf("no GPS in %s (id=%s)", path, id)
			return nil
		}
		err = store.UpdatePhoto(id, func(p *manifest.Photo) {
			p.Lat = lat
			p.Lon = lon
		})
		if err != nil {
			log.Printf("update %s: %v", id, err)
			return nil
		}
		updated++
		log.Printf("set %s -> %.6f,%.6f (from %s)", id, *lat, *lon, path)
		return nil
	})
	if walkErr != nil {
		log.Printf("walk: %v", walkErr)
	}
	log.Printf("done: scanned=%d matched=%d updated=%d no-gps=%d", scanned, matched, updated, missingGPS)
}

func isImage(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".tif", ".tiff", ".heic", ".webp":
		return true
	}
	return false
}
