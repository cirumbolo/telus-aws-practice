# CLAUDE.md — note.ms clone (AWS: EC2 + S3, Go)

## Project overview

A faithful clone of [note.ms](https://note.ms): a minimalist, anonymous,
URL-addressed plain-text notepad. Visit `/{slug}` and you get a single text area;
whatever you type auto-saves to that URL; reopening the URL restores the text. No
accounts, no formatting, no markdown — just a persistent pad per URL. Visiting the
root `/` redirects to a random slug.

This repo doubles as **AWS certification practice** (EC2, S3, IAM), so the
architecture deliberately favors hands-on AWS wiring over the simplest possible
implementation.

## Architecture

Two tiers: a static frontend on S3, and a Go JSON API on EC2 backed by a second
(private) S3 bucket for note storage.

```
Browser
  ├─ GET  S3 static site  (index.html + app.js + styles.css)      ← frontend
  └─ JS calls EC2 API (JSON over HTTP — see note below):
       GET /notes/{slug}   → { "text": "..." }   (200; missing key → 200 empty text)
       PUT /notes/{slug}   ← { "text": "..." }   (upsert; debounced auto-save)

EC2 (Go net/http) ──IAM instance role──▶ S3 notes bucket: notes/{slug}.txt
```

- **Frontend bucket:** static-website hosting (front with CloudFront + OAC later).
- **Notes bucket:** **private** — only the EC2 instance role may read/write it.
- **CORS:** the notes API must allow the S3 site's origin.
- **HTTP, not HTTPS:** S3 website endpoints are HTTP-only. An HTTP page calling
  an HTTP API is not mixed content, so this works. Putting CloudFront (HTTPS) in
  front of S3 while the API stays HTTP *is* active mixed content and is
  hard-blocked by browsers — see the phase-2 note in `infra/DEPLOY.md`.

### Tradeoff to keep in mind
S3 has no locking or transactions. That's fine for a single-editor-per-note pad, but
**not** for per-keystroke concurrent editing of the same note (last write wins). If
concurrent editing is ever needed, move the note store to DynamoDB — the API's
`store.go` is the only place that should change.

## Repository layout

```
/api/            Go API server
  main.go        wiring, config from env, backend selection, http.Server
  handler.go     GET/PUT /notes/{slug}, slug validation, CORS
  store.go       Store interface + ErrNotFound — the persistence seam
  store_fs.go    filesystem backend (local dev): data/{slug}.txt
  store_s3.go    S3 backend (aws-sdk-go-v2): notes/{slug}.txt
  *_test.go      handler contract, FSStore, and S3Store error-mapping tests
/web/            Static frontend (deployed to the S3 site bucket)
  index.html     the pad (single <textarea>)
  app.js         load-on-open + debounced auto-save (fetch); API_BASE seam
  styles.css     minimalist, full-viewport textarea
/infra/          DEPLOY.md — AWS console walkthrough (no IaC yet)
README.md
```

Adding a backend means implementing `Store` and wiring it into `newStore` in
`main.go`; handlers never change. DynamoDB would slot in the same way.

## Conventions

- **Slugs:** validate against `^[a-zA-Z0-9_-]{1,64}$` and reject anything else. This
  prevents S3 key injection / path traversal via the slug. Root `/` → redirect to a
  random valid slug.
- **Note size cap:** enforce a max request body on `PUT` (e.g. 100 KB).
- **Empty/missing note:** a missing note is an empty note — return `200` with
  `{"text": ""}`, not a `404`/error. A brand-new pad is a normal case. Every
  backend maps "missing" to `ErrNotFound` and the handler does the rest.
- **Missing objects in S3 — `NoSuchKey` vs `AccessDenied`:** S3 only returns
  `NoSuchKey` when the caller has `s3:ListBucket`. Without it, a missing object
  comes back as **`AccessDenied`**, because S3 deliberately hides existence from
  callers that can't list. Our least-privilege instance role grants only
  `s3:GetObject`/`s3:PutObject` on `notes/*`, so `AccessDenied` is the branch
  that fires in production — matching only `NoSuchKey` would turn every fresh
  pad into a `500`. `isNotFound` in `store_s3.go` handles both, and logs a
  warning on the `AccessDenied` path so a genuinely broken IAM policy (which
  otherwise presents as "every note is blank") stays visible in `journalctl`.
- **Go:** standard-library `net/http` + `aws-sdk-go-v2`; ships as a single static
  binary. Keep responses small JSON.
- **Secrets & config:** never hardcode AWS credentials. On EC2 use the **IAM instance
  role**; locally use the default credential chain (`AWS_PROFILE`). Bucket name and
  region come from env vars (`NOTES_BUCKET`, `AWS_REGION`) — never commit them.
- **Auto-save:** the client debounces writes (~500 ms–1 s idle) so typing doesn't fire
  an S3 `PUT` per keystroke.

## Local dev & build

```bash
cd api
NOTES_BUCKET=<dev-bucket> AWS_REGION=<region> go run .   # uses local AWS creds
go build -o note-api .                                    # single binary to ship to EC2
```

Serve `/web` with any static file server during frontend iteration; point its API base
URL at the local Go server.

## AWS deployment notes (cert practice)

- **EC2:** Amazon Linux; security group opens only the API port (+ SSH for admin);
  attach an **IAM instance profile** granting least-privilege
  `s3:GetObject` / `s3:PutObject` scoped to the notes bucket only.
- **S3:** one **private** notes bucket; one static-website frontend bucket (configure
  Block Public Access appropriately, or front it with CloudFront + Origin Access
  Control).
- **Frontend asset layout:** `index.html` references `/static/styles.css` and
  `/static/app.js`, which resolve locally via the Go server's `/static/` route.
  The S3 bucket must mirror that — upload the assets under a `static/` prefix,
  not flat, or the page loads unstyled with a dead script.
- **`API_BASE`:** stays `""` in git (same-origin local dev). The EC2 address is
  set only in the copy of `app.js` uploaded to S3, so an environment-specific IP
  never lands in version history.
- **Scaling later:** DynamoDB for concurrent editing; ALB + multiple instances or an
  Auto Scaling Group if the single instance isn't enough.

Full console walkthrough, with verification checkpoints and a failure-mode
table: [`infra/DEPLOY.md`](infra/DEPLOY.md).

## Verification (end-to-end, once code exists)

- `PUT /notes/{slug}` with some text, then `GET /notes/{slug}` returns the same text.
- Open `/{slug}` in a browser, type, reload — the text persists.
- A fresh slug returns empty text (not an error).
- An invalid slug (e.g. containing `/` or `..`) is rejected.
