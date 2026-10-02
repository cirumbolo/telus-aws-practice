#!/bin/sh
# Fetches the FuelIX API key from SSM Parameter Store into a tmpfs env file that
# note-api.service loads via EnvironmentFile=. Runs as ExecStartPre using the
# EC2 instance role — no credentials on disk. See infra/DEPLOY.md, step 6b.
#
# Needs AWS_REGION in the environment (set by the unit). On any failure it
# leaves no env file behind, so the API starts with summaries disabled (503)
# and the error shows up in `journalctl -u note-api`.
set -eu

PARAM="${FUELIX_KEY_PARAM:-/note-api/fuelix-api-key}"
OUT=/run/note-api/env

rm -f "$OUT"
umask 077
key=$(aws ssm get-parameter --name "$PARAM" --with-decryption \
  --query Parameter.Value --output text)
printf 'FUELIX_API_KEY=%s\n' "$key" > "$OUT"
