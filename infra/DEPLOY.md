# Deploying to AWS (EC2 + S3) — console walkthrough

Target architecture, per `CLAUDE.md`:

```
Browser
  ├─ GET  S3 static website (index.html + static/app.js + static/styles.css)
  └─ fetch → EC2 :8080 (Go API)
                  │ IAM instance role
                  ▼
            S3 notes bucket (private) — notes/{slug}.txt
```

Pick **one region** and use it for everything. Region mismatch is a top-3 cause
of failure.

> **Why plain HTTP everywhere?** S3 static website endpoints are HTTP-only. An
> HTTP page calling an HTTP API is *not* mixed content, so browsers allow it.
> The trap to avoid is putting CloudFront (HTTPS) in front of S3 while the API
> stays HTTP — that *is* active mixed content, hard-blocked with no override,
> and `web/app.js` swallows fetch errors, so it fails completely silently. See
> "Phase 2" at the end.

---

## Step 0 — AWS credentials (local)

Needed for Checkpoint 1 only; EC2 uses the instance role instead.

```bash
aws configure --profile telus     # or SSO
```

## Step 1 — Notes bucket (private)

S3 → **Create bucket**, e.g. `notems-notes-<yourname>`.

- **Block Public Access: leave all four settings ON.** This bucket is never public.
- Keep default encryption (SSE-S3).
- Versioning: optional, but a nice undo given the documented last-write-wins tradeoff.
- Do **not** pre-create the `notes/` prefix — `PutObject` creates it implicitly.

## Step 2 — IAM policy

IAM → Policies → **Create policy** → JSON. Name: `NoteMSNotesBucketRW`.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ReadWriteNoteObjects",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject"],
      "Resource": "arn:aws:s3:::REPLACE-BUCKET/notes/*"
    }
  ]
}
```

- The resource is the **object** ARN (`bucket/notes/*`), **not** the bucket ARN.
  Object actions scoped to a bucket ARN silently deny everything.
- **No `s3:ListBucket`** — deliberate. Without it S3 returns `AccessDenied`
  rather than `NoSuchKey` for a missing object, because it hides existence from
  callers that can't list. `isNotFound` in `api/store_s3.go` handles both, and
  logs a warning on the `AccessDenied` branch.
- Want the cleaner `NoSuchKey` behaviour instead? Add a second statement with
  `"Action": "s3:ListBucket"` on `arn:aws:s3:::REPLACE-BUCKET`. Try it both ways
  and watch the error change — it's the best hands-on IAM lesson here.

## Step 3 — IAM role + instance profile

IAM → Roles → **Create role** → **AWS service → EC2** → attach
`NoteMSNotesBucketRW`. Name: `NoteMSInstanceRole`.

EC2 consumes an **instance profile**, not a role directly. The console
auto-creates a profile of the same name; the CLI makes you do it manually
(`create-instance-profile` + `add-role-to-instance-profile`). Worth knowing for
the exam.

## Step 4 — Security group

EC2 → Security Groups → Create.

| Direction | Port | Source | Why |
| --------- | ---- | ------ | --- |
| Inbound | TCP 8080 | `0.0.0.0/0` | The browser connects to the API directly |
| Inbound | TCP 22 | **My IP only** | SSH admin — never `0.0.0.0/0` |
| Outbound | all | default | `dnf` + S3 API calls |

## Step 5 — Launch EC2

EC2 → **Launch instance**.

- AMI: **Amazon Linux 2023**.
- Instance type: `t4g.micro` (**arm64**) or `t3.micro` (**x86_64**).
  **Write down which — it determines `GOARCH` in step 6.**
- Key pair: create or select one.
- Security group: the one from step 4. **Auto-assign public IP: Enable.**
- **Advanced details → IAM instance profile: `NoteMSInstanceRole`.**
  Easy to miss; without it the app fails at runtime with `NoCredentialProviders`.
- Metadata version: **IMDSv2 required** (the AL2023 default). `aws-sdk-go-v2`
  speaks IMDSv2 natively. Note the default hop limit of 1 breaks credential
  retrieval from inside containers — not an issue here, but a classic gotcha.
- Consider an **Elastic IP** so the address survives stop/start (otherwise
  `API_BASE` in the uploaded `app.js` goes stale).

## Step 6 — Build and ship the binary

Match the **instance** architecture, not your laptop's:

```bash
cd api
GOOS=linux GOARCH=arm64 go build -o note-api .    # t4g.*
# GOOS=linux GOARCH=amd64 go build -o note-api .  # t3.* / t2.*

scp -i <key>.pem note-api ec2-user@<EC2-IP>:~/
ssh -i <key>.pem ec2-user@<EC2-IP> 'chmod +x note-api && sudo mv note-api /usr/local/bin/'
```

Pure stdlib + AWS SDK, so CGO is off and the binary is static — no glibc concerns.

Mismatched `GOARCH` gives `exec format error`.

## Step 7 — Run as a systemd service

Create `/etc/systemd/system/note-api.service`:

```ini
[Unit]
Description=note.ms clone API
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=ec2-user
Environment=NOTES_BUCKET=REPLACE-BUCKET
Environment=AWS_REGION=REPLACE-REGION
Environment=PORT=8080
Environment=ALLOW_ORIGIN=http://REPLACE-WEBSITE-ENDPOINT
ExecStart=/usr/local/bin/note-api
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now note-api
systemctl status note-api
journalctl -u note-api -f
```

- Setting `NOTES_BUCKET` alone selects the S3 backend (`STORE` is not needed).
  The startup log prints the resolved backend: `listening on :8080 (store=s3)`.
- **`ALLOW_ORIGIN` must match the website origin exactly** — scheme included,
  **no trailing slash**, no path. A mismatch produces a CORS failure that `curl`
  cannot reproduce.
- Chicken-and-egg: the endpoint comes from step 8. Do step 8 first, then set
  `ALLOW_ORIGIN` and `sudo systemctl restart note-api`.

## Step 8 — Frontend bucket + static website hosting

S3 → Create bucket, e.g. `notems-web-<yourname>`.

- **Block Public Access: turn all four OFF** — the deliberate contrast with the
  notes bucket. The console requires an acknowledgement.
- Properties → **Static website hosting: Enable**
  - Index document: `index.html`
  - **Error document: `index.html`** — this is what makes `/{slug}` render the
    pad. S3 has no clean-URL routing, so the response carries HTTP 404 with the
    right body; `app.js` reads the slug from `location.pathname`, so it works.
- Permissions → **Bucket policy** (set BPA off *first*, or the policy won't stick):

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "PublicReadForWebsite",
      "Effect": "Allow",
      "Principal": "*",
      "Action": "s3:GetObject",
      "Resource": "arn:aws:s3:::REPLACE-WEB-BUCKET/*"
    }
  ]
}
```

Website endpoint format (**copy the exact value the console shows**):

- `http://<bucket>.s3-website-<region>.amazonaws.com` — dash form (us-east-1, eu-west-1, …)
- `http://<bucket>.s3-website.<region>.amazonaws.com` — dot form (us-east-2, eu-central-1, …)

Not to be confused with the REST endpoint `<bucket>.s3.<region>.amazonaws.com`,
which does HTTPS but has no index/error-document routing.

## Step 9 — Upload the frontend

`web/index.html` references `/static/styles.css` and `/static/app.js`. Locally
those resolve via the Go server's `/static/` route (`api/handler.go`). **On S3
the layout must match, so assets go under a `static/` prefix** — uploading flat
gives an unstyled page with a dead script.

```
s3://REPLACE-WEB-BUCKET/
  index.html            ← from web/index.html
  static/
    app.js              ← from web/app.js  (edited, see below)
    styles.css          ← from web/styles.css
```

In the **uploaded copy of `app.js` only**, set the API base to the EC2 address:

```js
const API_BASE = "http://<EC2-IP>:8080";   // no trailing slash
```

Git keeps `API_BASE = ""` so local dev (same-origin) keeps working untouched.

> No S3 **bucket CORS** configuration is needed. Bucket CORS governs browser
> access to S3 objects; the cross-origin traffic here goes to the EC2 API, which
> sets CORS headers itself via `ALLOW_ORIGIN`.

---

## Verification checkpoints

Gate each stage. `app.js` swallows fetch errors, so **keep DevTools → Network
open** — otherwise every misconfiguration looks identical ("the pad is empty").

**0 — Unit tests, no AWS**
```bash
cd api && go vet ./... && go test ./...     # 13 passing
```

**1 — Local binary against the real bucket** (highest value: isolates the S3
store from all EC2/CORS concerns)
```bash
cd api
AWS_PROFILE=telus AWS_REGION=<region> NOTES_BUCKET=<notes-bucket> go run .
# → listening on :8080 (store=s3)

curl -X PUT localhost:8080/notes/abc12 \
  -H 'Content-Type: application/json' -d '{"text":"hello s3"}'   # 204
curl localhost:8080/notes/abc12                                  # {"text":"hello s3"}
curl localhost:8080/notes/brandnew                               # {"text":""} — not a 500
```
Confirm `notes/abc12.txt` exists in the S3 console with content type `text/plain`.

**2 — EC2 via curl** (proves instance profile → IAM → S3, plus the security group)
```bash
curl -X PUT http://<EC2-IP>:8080/notes/ec2test \
  -H 'Content-Type: application/json' -d '{"text":"from ec2"}'
curl http://<EC2-IP>:8080/notes/ec2test

# CORS preflight — expect 204 + Access-Control-Allow-Origin
curl -i -X OPTIONS -H 'Origin: http://<website-endpoint>' \
  -H 'Access-Control-Request-Method: PUT' http://<EC2-IP>:8080/notes/ec2test
```
If it fails: SSH in and `curl localhost:8080/...`. Works there = security group.
Fails there = IAM (check `journalctl -u note-api`).

**3 — Static site loads**
Open the website endpoint. In DevTools confirm `static/styles.css` and
`static/app.js` return **200, not 404**.

**4 — Browser end-to-end**
Open `http://<website-endpoint>/abc12` — the textarea shows the text from
checkpoint 2 (GET + CORS ✓). Type, wait >800 ms, reload — it persists (PUT ✓).
Open a fresh slug — empty, no error.

**5 — `CLAUDE.md` acceptance criteria**
PUT/GET roundtrip ✓ · persist on reload ✓ · fresh slug empty ✓ · invalid slug
rejected (400) ✓

---

## Common failure points

| Symptom | Cause |
| ------- | ----- |
| `NoCredentialProviders` in the log | Instance profile not attached at launch |
| `exec format error` | `GOARCH` doesn't match the instance architecture |
| Every request `AccessDenied` | Policy resource is the bucket ARN, not `bucket/notes/*` |
| Every note reads blank | IAM policy missing `s3:GetObject` — check `journalctl` for the AccessDenied warning |
| CORS error in browser, `curl` fine | `ALLOW_ORIGIN` trailing slash or scheme mismatch |
| `PermanentRedirect` / `BucketRegionError` | Region mismatch between bucket and `AWS_REGION` |
| Browser hangs, works over SSH | Security group missing 8080 inbound |
| Worked yesterday, dead today | Public IP changed on stop/start — use an Elastic IP |
| Bucket policy won't save | Block Public Access still on for the web bucket |
| Unstyled page, no saving | Assets not under the `static/` prefix |
| `ssh: connect ... port 22: Operation timed out`, but port 8080 works fine from the same machine | Your current public IP doesn't match the SSH rule's source — re-check with `curl -s https://checkip.amazonaws.com` and update the rule (common after switching networks) |
| Browser blocked with a proxy "Web Page Blocked" page, or a port hangs only on one network | Corporate/office network blocking non-standard outbound ports — retry from a phone hotspot or home network |

---

## Cost safety net — AWS Budgets stop action

Given this is a learning/cert-practice account, add a budget with an
**action**, not just an alert, so a forgotten running instance can't run up a
bill unattended:

1. IAM → Roles → Create role → **Custom trust policy**:
   ```json
   {
     "Version": "2012-10-17",
     "Statement": [{
       "Effect": "Allow",
       "Principal": { "Service": "budgets.amazonaws.com" },
       "Action": "sts:AssumeRole"
     }]
   }
   ```
   Attach a permissions policy allowing `ec2:StopInstances` and
   `ec2:DescribeInstances`. Name it e.g. `BudgetsStopEC2Role`.
2. Billing → Budgets → create a budget (e.g. $1 fixed) → add an **action** →
   Target = EC2 instances (running) → Action = Stop → role =
   `BudgetsStopEC2Role`. Also add an email alert on the same threshold.

Caveats to know going in — this is a backstop, not a hard cap:
- Budgets evaluates spend a few times a day, not in real time — you can exceed
  the threshold before it fires.
- It only stops EC2/RDS compute. It does **not** touch S3 storage/request
  charges, and a stopped instance's EBS volume keeps billing until terminated.
- **The only thing that guarantees $0 ongoing cost is manually terminating the
  EC2 instance and deleting the buckets when you're done for the session** —
  treat the budget action as a safety net for "I forgot", not a substitute for
  tearing down.

## Worked example — one completed deployment (2026-08-28, us-east-2)

Kept here as a shape-of-the-thing reference; real bucket names, IPs, and IDs
from that run are deliberately omitted since bucket names are effectively
public identifiers once written down and the notes bucket is meant to stay
obscure. Use your own values — don't reuse someone else's bucket names.

| Item | Value |
| ---- | ----- |
| Region | `us-east-2` (Ohio) — **dot-form** website endpoint, not dash-form |
| Notes bucket (private) | *(project-specific — keep out of docs/commits)* |
| Web bucket (public, static site) | *(project-specific; fine to share since it's public)* |
| Website endpoint | `http://<web-bucket>.s3-website.us-east-2.amazonaws.com` |
| Instance type | `t3.micro` → `GOARCH=amd64` |
| IAM policy | `NoteMSNotesBucketRW` |
| IAM instance role | *(project-specific role name)* |
| Security group | *(project-specific security group)* |

**Gotcha hit during this run: SSH timeout from a *different* network than the
one used to create the security group.** The inbound SSH rule was scoped to
the IP address captured at SG-creation time. Testing later from a different
network (home vs. office) presented a different public IP, so port 22 timed
out while port 8080 (open to `0.0.0.0/0`) kept working fine from the same
machine — the asymmetry between "one port works, one doesn't, same source" is
the tell. Fix: re-check your current public IP (`curl -s
https://checkip.amazonaws.com`) and update the SSH rule's source to match (or
use the console's "My IP" button) whenever you switch networks. Network ACLs
were *not* the cause here — the subnet NACL was left at its default allow-all,
so that's usually not worth checking first.

Corporate/office networks may also transparently block or intercept outbound
traffic to non-standard ports entirely (seen here as an HTTP proxy block page
on port 8080, and a bare connection timeout on port 22) — if a browser test or
SSH hangs or gets intercepted only on one network, retry from a phone hotspot
or home connection before assuming the AWS-side config is wrong.

## Phase 2 (optional) — CloudFront, two origins

Once the HTTP version works end to end, put CloudFront in front with **two
origins**: the S3 bucket for static files, the EC2 instance for `/notes/*`.

Benefits: one HTTPS origin (free `*.cloudfront.net` certificate, no domain
needed), **no CORS at all** (`API_BASE` goes back to `""`, `ALLOW_ORIGIN` unset),
and the error-document status-code wart disappears via a custom error response.

Watch out for: the `/notes/*` cache behaviour must use **`CachingDisabled`** and
allow all HTTP methods, or GETs get cached (stale notes) and PUTs are rejected.
Distribution changes take 5–15 minutes to deploy, which is why this comes
*after* the fast-iteration HTTP version.
