#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ENV_FILE="${REPO_ROOT}/.env"
SECRETS_DIR="${REPO_ROOT}/.secrets"
CERT_FILE="${SECRETS_DIR}/sealed-secrets.crt"
PLATFORM_SECRETS_DIR="${REPO_ROOT}/deploy/platform/secrets"

command -v kubectl >/dev/null 2>&1 || { echo "Error: kubectl is not installed" >&2; exit 1; }
command -v kubeseal >/dev/null 2>&1 || { echo "Error: kubeseal is not installed" >&2; exit 1; }

# Ensure sealing key and certificate exist
if [ ! -f "${CERT_FILE}" ]; then
  echo "Sealing certificate ${CERT_FILE} not found. Running gen-sealing-key.sh..."
  "${SCRIPT_DIR}/gen-sealing-key.sh"
fi

if [ ! -f "$ENV_FILE" ]; then
  echo "Error: $ENV_FILE does not exist. Run 'cp .env.example .env' and configure it." >&2
  exit 1
fi

# Function to extract a variable value from .env line-by-line without sourcing
get_env_var() {
  local target_key="$1"
  local val=""
  while IFS= read -r line || [ -n "$line" ]; do
    # Trim leading whitespace
    local trimmed="${line#"${line%%[![:space:]]*}"}"
    trimmed="${trimmed%"${trimmed##*[![:space:]]}"}"

    # Skip comments and blank lines
    if [ -z "$trimmed" ] || [[ "$trimmed" == \#* ]]; then
      continue
    fi

    # Check prefix
    if [[ "$trimmed" == "${target_key}="* ]]; then
      val="${trimmed#*=}"
      # Strip surrounding double or single quotes
      if [[ "$val" =~ ^\"(.*)\"$ ]]; then
        val="${BASH_REMATCH[1]}"
      elif [[ "$val" =~ ^\'(.*)\'$ ]]; then
        val="${BASH_REMATCH[1]}"
      fi
      printf "%s" "$val"
      return 0
    fi
  done < "$ENV_FILE"
  return 0
}

# If GRAFANA_ADMIN_PASSWORD is missing, generate and append it to .env
GRAFANA_ADMIN_PASSWORD="$(get_env_var "GRAFANA_ADMIN_PASSWORD")"
if [ -z "$GRAFANA_ADMIN_PASSWORD" ]; then
  NEW_PASSWORD="$(openssl rand -base64 18)"
  printf "\n# Grafana admin credentials\nGRAFANA_ADMIN_PASSWORD=%s\n" "$NEW_PASSWORD" >> "$ENV_FILE"
  GRAFANA_ADMIN_PASSWORD="$NEW_PASSWORD"
  unset NEW_PASSWORD
fi

# Helper function to seal a Kubernetes secret offline
# Arguments:
#   $1: namespace
#   $2: secret name
#   $3..: secret key=value pairs
seal_secret() {
  local namespace="$1"
  local secret_name="$2"
  shift 2

  local ns_dir="${PLATFORM_SECRETS_DIR}/${namespace}"
  mkdir -p "${ns_dir}"

  # Ensure namespace.yaml exists per namespace
  local ns_file="${ns_dir}/namespace.yaml"
  cat <<EOF > "${ns_file}"
apiVersion: v1
kind: Namespace
metadata:
  name: ${namespace}
EOF

  local from_literal_args=()
  for item in "$@"; do
    local secret_key="${item%%=*}"
    local secret_val="${item#*=}"
    from_literal_args+=( "--from-literal=${secret_key}=${secret_val}" )
  done

  local sealed_file="${ns_dir}/${secret_name}.sealed.yaml"
  kubectl create secret generic "${secret_name}" \
    --namespace "${namespace}" \
    "${from_literal_args[@]}" \
    --dry-run=client -o yaml | \
  kubeseal \
    --cert "${CERT_FILE}" \
    --format yaml \
    --controller-name sealed-secrets-controller \
    --controller-namespace kube-system > "${sealed_file}"

  echo "Generated sealed secret: deploy/platform/secrets/${namespace}/${secret_name}.sealed.yaml"
}

# ------------------------------------------------------------------------------
# Secrets Table
# Add new secrets here as: seal_secret <namespace> <secret_name> <key1=val1> ...
# ------------------------------------------------------------------------------

# 1. Grafana admin secret (namespace: monitoring, name: grafana-admin)
seal_secret "monitoring" "grafana-admin" \
  "admin-user=admin" \
  "admin-password=${GRAFANA_ADMIN_PASSWORD}"

echo "All secrets successfully sealed offline."
