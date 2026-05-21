# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Personal photography portfolio for **masnun.photos**. Two halves:

1. **`site/`** — static gallery deployed to GitHub Pages. Vanilla HTML/CSS/JS, no build step. Reads `site/data/photos.json` at runtime.
2. **`admin/`** — local-only Go HTTP server. Uploads photos to Cloudflare R2, generates resized variants, edits the manifest. Never deployed; runs on the operator's machine.

Photo binaries live in R2, not in the repo. The repo only stores the manifest plus markup/CSS/JS.

## Commands

```bash
# Backend (run from repo root)
make tidy         # cd admin && go mod tidy
make admin        # build admin binary at admin/admin
make run          # admin serve   -> http://127.0.0.1:7777 (admin UI)
make classify     # admin classify -dir ../photos -> admin/classifications.tsv
make review       # admin review  -> http://127.0.0.1:7778 (verify/edit TSV with thumbs)
make ingest       # admin ingest  -tsv classifications.tsv (upload + register)
make site-serve   # python3 -m http.server 8000 in site/

# Single Go test (when tests exist)
cd admin && go test ./manifest -run TestName -v
```

The admin binary is a subcommand CLI: `admin serve | classify | ingest`. Always pass the
subcommand explicitly when calling `go run .` directly. Older invocations without a
subcommand will print usage and exit non-zero.

Required env vars (load from `.env`, see `.env.example`):
`R2_ACCOUNT_ID`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`, `R2_BUCKET`, `R2_PUBLIC_URL`.
`ANTHROPIC_API_KEY` is required only for `classify`.

`R2_PUBLIC_URL` is the public origin for the bucket (custom domain like `https://photos.masnun.com` or the `pub-xxx.r2.dev` Cloudflare-issued URL). All photo URLs in the manifest are constructed by joining this with the object key.

## Architecture

### Data model — single source of truth: `site/data/photos.json`

Three top-level arrays: `photos`, `genres`, `collections`. Photos reference taxonomies by slug. The Go types in `admin/manifest/types.go` are authoritative; both frontend JS and admin UI must match the JSON shape produced by `manifest.Store.save()`.

A photo has three URL variants written at upload time: `thumb` (400px fit), `web` (1600px fit), `full` (original). Variants are uploaded to R2 under `photos/<uuid>/{thumb,web,full}.<ext>`.

### Bulk classify + ingest pipeline

Two-step workflow for backfilling many photos at once. Both subcommands live in `admin/cmd/`.

1. **`admin classify -dir <photos_dir>`** walks the directory recursively, resizes each image to 1568px JPEG (`images.ResizeForVision`), and POSTs to the Anthropic Messages API (`admin/classifier/claude.go`, raw HTTP, no SDK dep). Each response is parsed as strict JSON `{genres, caption, collection_hint}`, normalized to kebab-case slugs, and appended to `admin/classifications.tsv`. Resumable: existing rows are skipped on re-run (keyed by relative path). Concurrency defaults to 4; 429/5xx are retried with exponential backoff capped at 30s.

2. **`admin ingest -tsv classifications.tsv`** reads the (possibly edited) TSV, deduplicates against the manifest via `Photo.SourceHash` (SHA-256 of the source bytes), and for each new row: re-processes the original through `images.Process`, extracts EXIF, uploads thumb/web/full to R2, and appends a `manifest.Photo`. Unknown genre/collection slugs are auto-created with title-cased names so the operator can edit them later in the admin UI. A hash drift warning is logged if the original file changed between classify and ingest.

The split is deliberate: classify is the expensive remote call, ingest is the irreversible upload. Always review/edit `classifications.tsv` between the two — that is the only human checkpoint.

**`admin review`** is the visual checkpoint UI between classify and ingest. It loads the TSV in-memory, serves on-the-fly 600px JPEG thumbs from the originals directory (`/thumb/<rel>`) and the full image (`/full/<rel>`, opens in new tab). Each card lets the operator edit genres / caption / collection and PATCH back; rows can be Skip-removed entirely (won't be ingested). Atomic save via tmp + rename, same pattern as the manifest store. Path-traversal guard rejects any `<rel>` that resolves outside the configured photos dir. Don't run `review` and `classify` simultaneously — classify appends to the TSV while review holds an in-memory copy and would overwrite new rows on save. Run review after classify completes (or use the Reload button after classify finishes).

### Admin server flow (Go)

`admin/main.go` wires three components:
- `manifest.Store` — read/write `photos.json` with an RWMutex; atomic save via `tmp + rename`. All mutations go through this store; never edit the JSON by hand while the server is running.
- `storage.R2` — S3-compatible client pointed at `https://<account>.r2.cloudflarestorage.com` with `auto` region and path-style URLs.
- `images.Process` — single decode, two resizes via `disintegration/imaging` (Lanczos). Returns thumb + web bytes plus original bytes and source dimensions.

Routing uses Go 1.22 stdlib mux with method patterns (`GET /api/photos`) and `{id}`/`{slug}` path values. The embedded admin UI (`admin/web/`) is served at `/` via `embed.FS`.

Bound to `127.0.0.1` only. There is no auth — never expose the admin port to the network.

### Frontend (no build)

Pages: `site/index.html` (genre + collection tiles), `site/genre/index.html?s=<slug>`, `site/collection/index.html?s=<slug>`, `site/photo/index.html?id=<uuid>`. All four fetch `/data/photos.json` and render client-side via helpers in `site/assets/js/app.js`.

The query-string routing is deliberate: avoids needing a build step or per-genre HTML files. Trade-off is uglier URLs and a single fetch on every page load; acceptable for a personal portfolio where the manifest is small.

### Deploy

`.github/workflows/deploy.yml` uploads `site/` as a Pages artifact on push to `main`. `site/CNAME` configures the custom domain. The admin half is never built or deployed in CI.

The publish loop: run admin locally → upload/edit → commit the changed `site/data/photos.json` → push → Pages rebuilds. R2 objects are pushed directly by the admin server, not via git.

## Conventions

- Module path is `github.com/masnun/photos/admin`. If you move the repo, update `go.mod` and the internal imports together.
- Slugs are user-defined kebab-case; the admin UI does not auto-generate them. Photos reference taxonomies by slug, so renaming a slug requires updating every photo that references it (no migration helper exists yet).
- Deleting a photo via the admin also deletes the three R2 objects. Deleting a genre/collection only removes the taxonomy entry — photo references are left dangling on purpose so the operator can reassign rather than lose data.
