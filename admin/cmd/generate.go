package cmd

import (
	"flag"
	"log"

	"github.com/masnun/photos/admin/generate"
)

func Generate(args []string) {
	fs := flag.NewFlagSet("generate", flag.ExitOnError)
	manifestPath := fs.String("manifest", "../site/data/photos.json", "path to photos.json manifest")
	outDir := fs.String("out", "../site", "output directory (static site root)")
	fs.Parse(args)

	if err := generate.Run(generate.Options{ManifestPath: *manifestPath, OutDir: *outDir}); err != nil {
		log.Fatalf("generate: %v", err)
	}
	log.Printf("generated static site at %s from %s", *outDir, *manifestPath)
}
