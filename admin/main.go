package main

import (
	"fmt"
	"os"

	"github.com/masnun/photos/admin/cmd"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		cmd.Serve(os.Args[2:])
	case "classify":
		cmd.Classify(os.Args[2:])
	case "review":
		cmd.Review(os.Args[2:])
	case "ingest":
		cmd.Ingest(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: admin <command> [flags]

commands:
  serve      run admin HTTP server (browser-based upload/edit UI)
  classify   walk a directory, call Claude vision, write classifications.tsv
  review     open browser UI to verify/edit classifications.tsv with thumbnails
  ingest     read classifications.tsv, upload originals to R2, register in manifest

env vars:
  R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, R2_BUCKET, R2_PUBLIC_URL
  ANTHROPIC_API_KEY (classify only)
`)
}
