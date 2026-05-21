.PHONY: tidy admin run classify review ingest reclassify generate site-serve

tidy:
	cd admin && go mod tidy

admin:
	cd admin && go build -o admin .

# run admin HTTP server
run:
	cd admin && set -a; [ -f ../.env ] && . ../.env; set +a; go run . serve -manifest ../site/data/photos.json

# AI-classify all originals under ../photos -> admin/classifications.tsv (resumable)
classify:
	cd admin && set -a; [ -f ../.env ] && . ../.env; set +a; go run . classify -dir ../photos

# visual review UI for admin/classifications.tsv (thumbs + inline edit)
review:
	cd admin && go run . review -tsv classifications.tsv -dir ../photos

# upload + register everything from admin/classifications.tsv
ingest:
	cd admin && set -a; [ -f ../.env ] && . ../.env; set +a; go run . ingest -dir ../photos -manifest ../site/data/photos.json

# re-classify all photos in manifest against current genre list (locked)
reclassify:
	cd admin && set -a; [ -f ../.env ] && . ../.env; set +a; go run . reclassify -manifest ../site/data/photos.json

# preview reclassify changes without writing to manifest
reclassify-dry:
	cd admin && set -a; [ -f ../.env ] && . ../.env; set +a; go run . reclassify -manifest ../site/data/photos.json -dry-run

# render static HTML pages into site/ from photos.json
generate:
	cd admin && go run . generate -manifest ../site/data/photos.json -out ../site

# preview the static site locally
site-serve:
	cd site && python3 -m http.server 8000
