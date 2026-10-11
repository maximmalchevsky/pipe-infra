#!/usr/bin/env bash
# Read-only Proxmox API client: performs GET requests only.
# Usage: scripts/pve-get.sh /nodes   (path relative to /api2/json)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROXMOX_VE_API_TOKEN="$(sed -n 's/^PROXMOX_VE_API_TOKEN=//p' "$ROOT/.env" | head -n1)"
PROXMOX_VE_ENDPOINT="$(sed -n 's/^PROXMOX_VE_ENDPOINT=//p' "$ROOT/.env" | head -n1)"

path="${1:?usage: $0 /api/path}"
[[ "$path" == /* ]] || { echo "path must start with /" >&2; exit 1; }

exec curl -sS -k --fail-with-body -X GET \
  -H "Authorization: PVEAPIToken=${PROXMOX_VE_API_TOKEN}" \
  "${PROXMOX_VE_ENDPOINT%/}/api2/json${path}"
