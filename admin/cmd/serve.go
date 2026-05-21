package cmd

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/masnun/photos/admin/manifest"
	"github.com/masnun/photos/admin/server"
	"github.com/masnun/photos/admin/storage"
)

func Serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7777", "listen address (keep on loopback — no auth)")
	manifestPath := fs.String("manifest", "../site/data/photos.json", "path to photos.json manifest")
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

	log.Printf("admin listening on http://%s (manifest=%s)", *addr, *manifestPath)
	if err := http.ListenAndServe(*addr, server.New(store, r2)); err != nil {
		log.Fatal(err)
	}
}

func r2Config() storage.Config {
	return storage.Config{
		AccountID:       mustEnv("R2_ACCOUNT_ID"),
		AccessKeyID:     mustEnv("R2_ACCESS_KEY_ID"),
		SecretAccessKey: mustEnv("R2_SECRET_ACCESS_KEY"),
		Bucket:          mustEnv("R2_BUCKET"),
		PublicURL:       mustEnv("R2_PUBLIC_URL"),
	}
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env var %s", k)
	}
	return v
}
