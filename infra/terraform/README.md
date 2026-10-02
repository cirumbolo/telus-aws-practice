# Terraform

Codifies the stack from [`../DEPLOY.md`](../DEPLOY.md): notes + web buckets, EC2
instance role/policy, security group, instance, Elastic IP. The live resources
are **imported**, not recreated (`imports.tf`).

## Bootstrap (once)
```bash
cd infra/bootstrap
terraform init && terraform apply -var state_bucket=<unique-name>
```

## Adopt the existing stack
```bash
cd infra/terraform
cp terraform.tfvars.example terraform.tfvars   # fill in real IDs (gitignored)
terraform init -backend-config="bucket=<state-bucket>" \
               -backend-config="key=note-ms/terraform.tfstate" \
               -backend-config="region=us-east-2"
terraform plan
```
**Do not apply until the plan shows imports only, with no replacement of the
instance, buckets, or role.** Fix HCL (not the cloud) until it's clean.

## File map
| File                | Owns                                                        |
| ------------------- | ----------------------------------------------------------- |
| `s3_notes.tf`       | private notes bucket, public-access block, SSE              |
| `s3_web.tf`         | public static-website bucket, website config, bucket policy |
| `iam_instance.tf`   | notes RW policy, instance role, instance profile            |
| `ec2.tf`            | security group, instance, Elastic IP + association          |
| `imports.tf`        | `import` blocks adopting the live resources                 |
| `backend.tf`        | S3 remote state (bucket/key/region passed at `init`)        |
| `variables.tf` / `terraform.tfvars.example` | inputs; copy the example to the gitignored `terraform.tfvars` |

## CI
`.github/workflows/pr-checks.yml` runs `terraform fmt -recursive -check` and
`terraform init -backend=false` + `validate` for both `infra/terraform` and
`infra/bootstrap` on every PR. It never plans or applies — run those locally.

## Things to know
- SSH (22) is open to `0.0.0.0/0` because GitHub-hosted runners deploy over SSH.
  Narrow it via `ssh_cidr` only once the deploy path changes.
- The instance ignores `ami`, `key_name`, `user_data`, `subnet_id`: the systemd
  unit is hand-written and the original key's private half is lost.
- The notes policy deliberately has no `s3:ListBucket` (see `CLAUDE.md`).
- Not yet codified (separate follow-ups): the FuelIX SSM parameter and its
  `ssm:GetParameter` policy, the budget and its stop action, the frontend CI IAM
  user (replace its long-lived keys with OIDC for `deploy-web.yml`).
