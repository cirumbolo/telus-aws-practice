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

## Things to know
- SSH (22) is open to `0.0.0.0/0` because GitHub-hosted runners deploy over SSH.
  Narrow it via `ssh_cidr` only once the deploy path changes.
- The instance ignores `ami`, `key_name`, `user_data`, `subnet_id`: the systemd
  unit is hand-written and the original key's private half is lost.
- The notes policy deliberately has no `s3:ListBucket` (see `CLAUDE.md`).
- The budget, its stop action, and OIDC for `deploy-web.yml` are separate follow-ups.
