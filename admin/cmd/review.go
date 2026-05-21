package cmd

import (
	"flag"
	"log"
	"net/http"

	"github.com/masnun/photos/admin/review"
)

func Review(args []string) {
	fs := flag.NewFlagSet("review", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7778", "listen address")
	tsv := fs.String("tsv", "classifications.tsv", "TSV path")
	dir := fs.String("dir", "../photos", "photos source directory")
	fs.Parse(args)

	h, err := review.New(*tsv, *dir)
	if err != nil {
		log.Fatalf("review init: %v", err)
	}
	log.Printf("review UI on http://%s  (tsv=%s, dir=%s)", *addr, *tsv, *dir)
	if err := http.ListenAndServe(*addr, h); err != nil {
		log.Fatal(err)
	}
}
