# Deploying to AWS (EC2 + S3) — console walkthrough

> **Terraform is now the source of truth** for this stack (see
> [`terraform/README.md`](terraform/README.md)). The console steps below remain
> as a reference for what each resource is and why.

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

IAM → Policies → **Create policy** → JSON. Name: `NoteMSNotesBucketRW`
(the live account may use a different name; same policy).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ReadWriteNoteObjects",
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:PutObject", "s3:CopyObject", "s3:DeleteObject"],
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
- **`s3:CopyObject`/`s3:DeleteObject` added 2026-09-28** for the note-rename
  feature: `S3Store.Rename` (`api/store_s3.go`) has no atomic move primitive
  in S3, so it copies to the new key then deletes the old one. Applied live
  as policy version `v2` on the notes policy via `aws iam
  create-policy-version --set-as-default` — no EC2 restart needed, IAM
  changes take effect immediately for the running instance role.

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

> The live stack deviates from "My IP only": once GitHub Actions deploys over
> SSH, port 22 has to be `0.0.0.0/0` (see "Continuous deploy" below and
> `terraform/README.md`). Start with My IP; widen only when you add CI.

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

## Step 6b — FuelIX API key in Parameter Store (optional, enables Summary)

The Summary button calls FuelIX from the API, so the key lives only on the
server side. Store it as an encrypted SSM parameter and let the instance role
read it at service start — nothing secret in git, the S3 buckets, or the unit
file.

1. **Create the parameter** (from a machine with admin creds; `read -rs` keeps the
   key out of your shell history):

   ```bash
   read -rs KEY
   aws ssm put-parameter --name /note-api/fuelix-api-key \
     --type SecureString --value "$KEY" --overwrite --region REPLACE-REGION
   unset KEY
   ```

   `SecureString` encrypts with the AWS-managed key `alias/aws/ssm` by default.
   Rotating the key later is the same `put-parameter --overwrite` plus
   `sudo systemctl restart note-api`.

2. **Grant read access.** IAM → Policies → Create policy, name
   `NoteMSFuelIXKeyRead`, then attach it to `NoteMSInstanceRole`:

   ```json
   {
     "Version": "2012-10-17",
     "Statement": [
       {
         "Sid": "ReadFuelIXKey",
         "Effect": "Allow",
         "Action": "ssm:GetParameter",
         "Resource": "arn:aws:ssm:REPLACE-REGION:REPLACE-ACCOUNT-ID:parameter/note-api/fuelix-api-key"
       }
     ]
   }
   ```

   - Scope to the one parameter ARN, no wildcard. In the ARN the parameter
     name's leading `/` is the separator after `parameter` — write
     `parameter/note-api/...`, not `parameter//note-api/...`.
   - The default `aws/ssm` key needs no extra `kms:Decrypt` statement. If you
     switch to a customer-managed key, add `kms:Decrypt` on that key's ARN or
     `GetParameter --with-decryption` fails with `AccessDeniedException`.

3. **Install the fetch script** on the instance (it is committed at
   `infra/fetch-fuelix-key.sh`):

   ```bash
   scp infra/fetch-fuelix-key.sh ec2-user@REPLACE-IP:/tmp/
   ssh ec2-user@REPLACE-IP 'sudo install -m 0755 -o root /tmp/fetch-fuelix-key.sh /usr/local/bin/'
   ```

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
# Optional — enables the Summary button (step 6b). Without the key the
# summary endpoint just returns 503. The URL and model aren't secret.
Environment=FUELIX_BASE_URL=https://api.fuelix.ai/v1
Environment=FUELIX_MODEL=claude-sonnet-4-5
RuntimeDirectory=note-api
RuntimeDirectoryMode=0700
# "-" = keep starting if the key can't be fetched (summaries stay disabled).
ExecStartPre=-/usr/local/bin/fetch-fuelix-key.sh
EnvironmentFile=-/run/note-api/env
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
- `FUELIX_API_KEY` is never in the unit file: `fetch-fuelix-key.sh` writes it
  to `/run/note-api/env` (tmpfs, `0600`, gone on stop/reboot) on every start.
  The instance needs outbound HTTPS to FuelIX (default security-group egress
  allows it). Verify the key loaded: `journalctl -u note-api -b | grep -i
  'ssm\|AccessDenied'` should be empty, and
  `curl -s -X POST localhost:8080/notes/<slug>/summary` should not say
  `summary not configured`.
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

**This step is now automated** by `.github/workflows/deploy-web.yml` — on every
push to `main` that touches `web/**`, it does the `sed` substitution above and
uploads all three files to the web bucket. Setup is documented under
"Frontend deploy" in the Continuous deploy section below. Manual upload is
still useful for a one-off test without pushing to `main`; the normal path is
merge to `main` → `deploy.yml` (API) and `deploy-web.yml` (frontend) run
automatically off the paths they each own.

---

## Verification checkpoints

Gate each stage. `app.js` swallows fetch errors, so **keep DevTools → Network
open** — otherwise every misconfiguration looks identical ("the pad is empty").

**0 — Unit tests, no AWS**
```bash
cd api && go vet ./... && go test ./...     # all passing
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
| Summary returns `503 summary not configured` on EC2 | Key fetch failed — `journalctl -u note-api -b` for the `ssm get-parameter` error: `AccessDeniedException` (policy/ARN/region mismatch), `ParameterNotFound`, or `fetch-fuelix-key.sh` not installed |
| `exec format error` | `GOARCH` doesn't match the instance architecture |
| Every request `AccessDenied` | Policy resource is the bucket ARN, not `bucket/notes/*` |
| Every note reads blank | IAM policy missing `s3:GetObject` — check `journalctl` for the AccessDenied warning |
| CORS error in browser, `curl` fine | `ALLOW_ORIGIN` trailing slash or scheme mismatch |
| `PermanentRedirect` / `BucketRegionError` | Region mismatch between bucket and `AWS_REGION` |
| Browser hangs, works over SSH | Security group missing 8080 inbound |
| Worked yesterday, dead today | Public IP changed on stop/start — use an Elastic IP (this instance has one, so this shouldn't recur unless the EIP is released) |
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

## Continuous deploy (GitHub Actions)

Three workflows live in `.github/workflows/`. Besides the two deploy pipelines
below, `pr-checks.yml` runs on every PR to `main`: `go vet`, `go test` with a
coverage summary, `gosec` static analysis, and `terraform fmt -check` +
`validate` for `infra/terraform` and `infra/bootstrap`. It only verifies; it
deploys nothing.

`.github/workflows/deploy.yml` automates steps 6–7 above (build, ship,
restart) on every push to `main` that touches `api/**`. It runs `go vet` +
`go test`, cross-compiles for `linux/amd64` (matching the `t3.micro` instance
type used so far — change this if a future instance is arm64/`t4g.*`), scp's
the binary over, and restarts the systemd service via SSH, finishing with a
curl health check against the running service.

### One-time setup before the first automated deploy — done (2026-09-28)

1. **Elastic IP allocated and associated** with the instance (address kept out of the docs). Without this, every stop/start changes the public IP,
   which silently breaks the `EC2_HOST` secret below — the exact stale-IP
   failure mode hit manually earlier in that session, just moved from the
   local SSH command to CI. An EIP stays associated with a *stopped*
   instance too, so powering the instance off between uses no longer
   requires touching this secret.
2. **SSH inbound rule widened to `0.0.0.0/0` on port 22.** GitHub Actions
   runners don't have a stable IP, so the previous "my IP only" rule
   couldn't admit them. Chose the simpler option over maintaining GitHub's
   rotating [Actions IP ranges](https://api.github.com/meta) — acceptable
   here since this is a low-value lab instance and password auth stays
   disabled (key-only). Reconsider for anything real.
3. **New dedicated CI key pair generated** (ed25519, stored locally
   outside the repo) — the original key pair's private key
   was lost (never saved to this machine), discovered when manual SSH
   auth failed. The new public key was appended to `ec2-user`'s
   `~/.ssh/authorized_keys` via a temporary EC2 Instance Connect session
   (doesn't require the lost key), so it persists across reboots
   independent of the original key pair.
4. **Repo secrets set** (`gh secret set`, not the console, but equivalent):
   - `EC2_HOST` = the Elastic IP
   - `EC2_SSH_KEY` = contents of the new CI private key (not
     the original `.pem`)
   - (the SSH user is hardcoded as `ec2-user` in the workflow, matching
     Amazon Linux — no secret needed for it)

### What it does NOT do yet

- No rollback on a failed health check — a bad deploy currently just fails
  the GitHub Actions run with the old binary already replaced; restoring the
  previous binary is a reasonable follow-up once the pipeline's proven out.
- Does not create the Elastic IP, security group rule, or secrets — those
  are manual one-time steps above, not automated by the workflow itself.

### Frontend deploy — `.github/workflows/deploy-web.yml`

Separate workflow, separate trigger path (`web/**`), because the frontend and
API deploy to different targets (S3 vs. EC2) on independent release cadences —
a `web/`-only change shouldn't rebuild/restart the API, and vice versa.

Automates Step 9 above: on every push to `main` touching `web/**`, it
substitutes `API_BASE` into a deploy copy of `app.js` (git keeps the source
`API_BASE = ""`) and uploads `index.html`/`static/app.js`/`static/styles.css`
to the web bucket via `aws s3 cp`.

**One-time setup done (2026-09-28):**

1. **Dedicated CI IAM user** with an inline policy:
   `s3:ListBucket` on the bucket, `s3:PutObject`/`s3:GetObject` on
   `bucket/*` — write access to this one bucket only, no broader S3 or IAM
   permissions. Access key pair generated via `aws iam create-access-key`.
2. **Repo secrets set:**
   - `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` — the CI user's key pair.
   - `S3_WEB_BUCKET` = the web bucket name
   - `AWS_REGION` = the deployment region
   - Reuses the existing `EC2_HOST` secret to build `API_BASE` at deploy
     time, so the EC2 address has one source of truth across both workflows.

**What it does NOT do yet:** no cache invalidation (not needed — no
CloudFront in front yet, see Phase 2), no rollback on upload failure.

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
