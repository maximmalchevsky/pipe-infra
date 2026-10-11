#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="${REPO_ROOT}/.env"
OUTPUT_FILE="${REPO_ROOT}/infra/terraform/env.auto.tfvars"

if [ ! -f "$ENV_FILE" ]; then
  echo "Error: $ENV_FILE does not exist. Run 'cp .env.example .env' and edit it." >&2
  exit 1
fi

umask 077

PROXMOX_ENDPOINT=""
PROXMOX_API_TOKEN=""
PROXMOX_INSECURE="true"

TMP_VARS="$(mktemp "${TMPDIR:-/tmp}/tfvars.XXXXXX")"
trap 'rm -f "$TMP_VARS"' EXIT

while IFS= read -r line || [ -n "$line" ]; do
  # Trim leading whitespace
  trimmed="${line#"${line%%[![:space:]]*}"}"

  # Skip blank lines and comments
  if [ -z "$trimmed" ] || [[ "$trimmed" == \#* ]]; then
    continue
  fi

  # Skip lines without =
  if [[ "$trimmed" != *"="* ]]; then
    continue
  fi

  # Split on first '='
  key="${trimmed%%=*}"
  val="${trimmed#*=}"

  # Trim whitespace from key
  key="${key#"${key%%[![:space:]]*}"}"
  key="${key%"${key##*[![:space:]]}"}"

  # Trim whitespace from val
  val="${val#"${val%%[![:space:]]*}"}"
  val="${val%"${val##*[![:space:]]}"}"

  case "$key" in
    PROXMOX_VE_ENDPOINT)
      if [[ "$val" =~ ^\"(.*)\"$ ]] || [[ "$val" =~ ^\'(.*)\'$ ]]; then
        val="${BASH_REMATCH[1]}"
      fi
      PROXMOX_ENDPOINT="$val"
      ;;
    PROXMOX_VE_API_TOKEN)
      if [[ "$val" =~ ^\"(.*)\"$ ]] || [[ "$val" =~ ^\'(.*)\'$ ]]; then
        val="${BASH_REMATCH[1]}"
      fi
      PROXMOX_API_TOKEN="$val"
      ;;
    PROXMOX_VE_INSECURE)
      if [[ "$val" =~ ^\"(.*)\"$ ]] || [[ "$val" =~ ^\'(.*)\'$ ]]; then
        val="${BASH_REMATCH[1]}"
      fi
      if [[ "$val" == "false" || "$val" == "FALSE" || "$val" == "0" ]]; then
        PROXMOX_INSECURE="false"
      else
        PROXMOX_INSECURE="true"
      fi
      ;;
    TF_VAR_*)
      var_name="${key#TF_VAR_}"
      if [[ "$val" == \[* ]] || [[ "$val" == \{* ]]; then
        echo "${var_name} = ${val}" >> "$TMP_VARS"
      elif [[ "$val" =~ ^-?[0-9]+(\.[0-9]+)?$ ]]; then
        echo "${var_name} = ${val}" >> "$TMP_VARS"
      elif [[ "$val" == "true" || "$val" == "false" ]]; then
        echo "${var_name} = ${val}" >> "$TMP_VARS"
      else
        if [[ "$val" =~ ^\"(.*)\"$ ]] || [[ "$val" =~ ^\'(.*)\'$ ]]; then
          val="${BASH_REMATCH[1]}"
        fi
        escaped_val="${val//\\/\\\\}"
        escaped_val="${escaped_val//\"/\\\"}"
        echo "${var_name} = \"${escaped_val}\"" >> "$TMP_VARS"
      fi
      ;;
  esac
done < "$ENV_FILE"

escaped_endpoint="${PROXMOX_ENDPOINT//\\/\\\\}"
escaped_endpoint="${escaped_endpoint//\"/\\\"}"

escaped_token="${PROXMOX_API_TOKEN//\\/\\\\}"
escaped_token="${escaped_token//\"/\\\"}"

cat <<EOF > "$OUTPUT_FILE"
# Generated from .env by scripts/env-to-tfvars.sh — do not edit

proxmox_endpoint = "${escaped_endpoint}"
proxmox_api_token = "${escaped_token}"
proxmox_insecure = ${PROXMOX_INSECURE}
EOF

cat "$TMP_VARS" >> "$OUTPUT_FILE"
chmod 0600 "$OUTPUT_FILE"
terraform fmt "$OUTPUT_FILE" >/dev/null 2>&1 || true
