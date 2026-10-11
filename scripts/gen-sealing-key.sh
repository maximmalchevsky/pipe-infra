#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SECRETS_DIR="${REPO_ROOT}/.secrets"
KEY_FILE="${SECRETS_DIR}/sealed-secrets.key"
CRT_FILE="${SECRETS_DIR}/sealed-secrets.crt"

umask 077
mkdir -p "${SECRETS_DIR}"

# Idempotency check: never overwrite existing keys
if [ -f "${KEY_FILE}" ] && [ -f "${CRT_FILE}" ]; then
  echo "Sealing key and certificate already exist in ${SECRETS_DIR}, skipping generation."
  exit 0
fi

if [ -f "${KEY_FILE}" ] || [ -f "${CRT_FILE}" ]; then
  echo "Error: Incomplete sealing key pair found in ${SECRETS_DIR}. Either both or neither must exist." >&2
  echo "Refusing to overwrite existing file. Inspect or remove the orphaned file manually." >&2
  exit 1
fi

echo "Generating Sealed Secrets key pair (RSA 4096, 10 years validity)..."
openssl req -x509 -nodes -newkey rsa:4096 \
  -keyout "${KEY_FILE}" \
  -out "${CRT_FILE}" \
  -subj "/CN=sealed-secret" \
  -days 3650 >/dev/null 2>&1

chmod 0600 "${KEY_FILE}"
chmod 0644 "${CRT_FILE}"

echo "Sealing key pair successfully generated in ${SECRETS_DIR}."
