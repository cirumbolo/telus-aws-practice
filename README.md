# telus-aws-practice

Projeto para treinar a certificacao do AWS e mentoria de Ideraldo para Guilherme.

A [note.ms](https://note.ms) clone: a minimalist, anonymous, URL-addressed
plain-text notepad. Visit `/{slug}`, type into the single text area, and it
auto-saves to that URL; reopening the URL restores the text. See `CLAUDE.md`
for the full architecture and the eventual AWS (EC2 + S3) target.

## Prerequisites

- Go 1.22+ (developed on 1.26). No AWS account or credentials needed for local
  dev — notes are stored on the local filesystem.

## Run locally

The Go server serves both the JSON API and the static frontend from a single
origin, so one command runs the whole app:

```bash
cd api
go run .
# → listening on :8080 (store=fs)
```

Then open <http://localhost:8080/> in a browser. The root redirects to a random
slug; type into the pad, wait ~1 second, and reload — the text persists. Open
any `/{slug}` (e.g. <http://localhost:8080/mynotes>) to get a fresh or existing
pad.

Notes are written as `{slug}.txt` under `api/data/` (created automatically, and
git-ignored).

## Configuration (env vars)

All optional for local dev — the defaults are geared to filesystem storage on a
single origin.

| Variable       | Default   | Purpose                                              |
| -------------- | --------- | ---------------------------------------------------- |
| `PORT`         | `8080`    | Port the server listens on.                          |
| `STORE`        | `fs`      | Backend selection. `s3` is a future seam (see below).|
| `NOTES_DIR`    | `./data`  | Directory for the filesystem store.                  |
| `WEB_DIR`      | `../web`  | Directory holding `index.html` + static assets.      |
| `ALLOW_ORIGIN` | *(unset)* | Enables CORS for a given origin (for a two-origin/S3 deployment). Left unset locally. |

Example:

```bash
PORT=9000 NOTES_DIR=/tmp/notes go run .
```

Setting `STORE=s3` (or `NOTES_BUCKET`) currently exits with "s3 store not yet
implemented" — the S3 backend is a documented seam, not built for the local
phase.

## API

- `GET /notes/{slug}` → `200 {"text":"..."}`. A missing note returns
  `200 {"text":""}` (never `404`) — a brand-new pad is a normal case.
- `PUT /notes/{slug}` ← `{"text":"..."}` → `204 No Content`. Upsert; request
  body capped at 100 KiB.

Slugs must match `^[a-zA-Z0-9_-]{1,64}$`; anything else is rejected with `400`.

### Quick check with curl

```bash
# PUT then GET roundtrip
curl -X PUT http://localhost:8080/notes/abc12 \
  -H 'Content-Type: application/json' -d '{"text":"hello world"}'   # → 204
curl http://localhost:8080/notes/abc12                              # → {"text":"hello world"}

# a fresh slug is empty, not an error
curl http://localhost:8080/notes/brandnew                           # → {"text":""}
```

## Build & test

```bash
cd api
go test ./...            # run the unit + handler tests
go vet ./...             # static checks
go build -o note-api .   # single static binary (shipped to EC2 later)
```
