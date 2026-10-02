# telus-aws-practice

Projeto para treinar a certificacao do AWS e mentoria de Ideraldo para Guilherme.

A [note.ms](https://note.ms) clone: a minimalist, anonymous, URL-addressed
plain-text notepad. Visit `/{slug}`, type into the single text area, and it
auto-saves to that URL; reopening the URL restores the text. See `CLAUDE.md`
for the full architecture. The app is deployed on AWS (EC2 + S3); the stack is
codified in Terraform and shipped by GitHub Actions (see
[Infrastructure & CI/CD](#infrastructure--cicd)).

Beyond the basic pad it supports renaming a note to a new slug and an
AI-generated summary of a note (the "Summary" button, backed by FuelIX).

## Repository layout

| Path          | What it is                                                       |
| ------------- | ---------------------------------------------------------------- |
| `api/`        | Go JSON API (`net/http`); `fs` and `s3` storage backends         |
| `web/`        | Static frontend (`index.html`, `app.js`, `styles.css`)           |
| `infra/`      | AWS walkthrough (`DEPLOY.md`), Terraform, FuelIX key fetch script |
| `.github/workflows/` | PR checks and the two deploy pipelines                    |

## Prerequisites

- Go 1.26+ (see `api/go.mod`). No AWS account or credentials needed for local
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
| `STORE`        | `fs`      | Backend selection: `fs` or `s3`. Unknown values are rejected. |
| `NOTES_DIR`    | `./data`  | Directory for the filesystem store.                  |
| `NOTES_BUCKET` | *(unset)* | S3 bucket for the `s3` store. Setting it selects `s3` regardless of `STORE`. |
| `AWS_REGION`   | *(unset)* | Region for the `s3` store (or take it from the AWS profile). |
| `WEB_DIR`      | `../web`  | Directory holding `index.html` + static assets.      |
| `ALLOW_ORIGIN` | *(unset)* | Enables CORS for a given origin (for a two-origin/S3 deployment). Left unset locally. |
| `FUELIX_API_KEY` | *(unset)* | Enables the Summary button (`POST /notes/{slug}/summary`). Unset → the endpoint returns 503. Requires the two vars below. |
| `FUELIX_BASE_URL` | *(unset)* | FuelIX OpenAI-compatible base URL; `/chat/completions` is appended. |
| `FUELIX_MODEL` | *(unset)* | Model name sent to FuelIX. |

Example:

```bash
PORT=9000 NOTES_DIR=/tmp/notes go run .
```

## Running against S3

The `s3` backend stores notes as `notes/{slug}.txt` in a private bucket. It uses
the default AWS credential chain, so the same binary works locally with a profile
and on EC2 with an instance role — no code difference:

```bash
cd api
AWS_PROFILE=<profile> AWS_REGION=<region> NOTES_BUCKET=<bucket> go run .
# → listening on :8080 (store=s3)
```

The API behaves identically to the filesystem backend, so the curl checks below
apply unchanged. See [`infra/DEPLOY.md`](infra/DEPLOY.md) for the full AWS
(EC2 + S3) deployment walkthrough.

`aws-sdk-go-v2` is the module's only external dependency; everything else is
standard library.

## API

- `GET /notes/{slug}` → `200 {"text":"..."}`. A missing note returns
  `200 {"text":""}` (never `404`) — a brand-new pad is a normal case.
- `PUT /notes/{slug}` ← `{"text":"..."}` → `204 No Content`. Upsert; request
  body capped at 100 KiB.
- `PATCH /notes/{slug}` ← `{"slug":"new-slug"}` → `204 No Content`. Renames
  (moves) the note. `404` if the source note doesn't exist, `409` if a note
  already exists at the new slug.
- `POST /notes/{slug}/summary` → `200 {"summary":"..."}`. Summarizes the
  *saved* note (the body is ignored, so the endpoint can't be used as a generic
  LLM proxy). `503` if FuelIX isn't configured, `502` if the upstream call fails.

Slugs (both in the path and in a rename body) must match
`^[a-zA-Z0-9_-]{1,64}$`; anything else is rejected with `400`.

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
go build -o note-api .   # single static binary (what gets shipped to EC2)
```

## Infrastructure & CI/CD

- **Walkthrough:** [`infra/DEPLOY.md`](infra/DEPLOY.md) explains each AWS
  resource (notes bucket, IAM role, security group, EC2, web bucket) and how to
  verify it.
- **Terraform:** [`infra/terraform/`](infra/terraform/README.md) is the source of
  truth for the live stack (imported, not recreated); `infra/bootstrap/` creates
  the remote-state bucket once.
- **GitHub Actions:**

  | Workflow        | Trigger                          | Does                                              |
  | --------------- | -------------------------------- | ------------------------------------------------- |
  | `pr-checks.yml` | pull request to `main`           | `go vet`, tests + coverage, `gosec`, `terraform fmt`/`validate` |
  | `deploy.yml`    | push to `main` touching `api/**` | test, build `linux/amd64`, ship over SSH, restart, health check |
  | `deploy-web.yml`| push to `main` touching `web/**` | inject `API_BASE`, upload to the web bucket       |
