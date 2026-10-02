# Bucket/key/region are passed at init so no account-specific names are committed:
#   terraform init -backend-config="bucket=<state-bucket>" \
#                  -backend-config="key=note-ms/terraform.tfstate" \
#                  -backend-config="region=us-east-2"
terraform {
  backend "s3" {
    use_lockfile = true
    encrypt      = true
  }
}
