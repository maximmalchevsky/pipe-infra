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

# If NOTIFIER_DB_PASSWORD is missing, generate and append it to .env
NOTIFIER_DB_PASSWORD="$(get_env_var "NOTIFIER_DB_PASSWORD")"
if [ -z "$NOTIFIER_DB_PASSWORD" ]; then
  NEW_DB_PASSWORD="$(LC_ALL=C tr -dc A-Za-z0-9 < /dev/urandom | head -c 32)"
  printf "\n# Notifier database password\nNOTIFIER_DB_PASSWORD=%s\n" "$NEW_DB_PASSWORD" >> "$ENV_FILE"
  NOTIFIER_DB_PASSWORD="$NEW_DB_PASSWORD"
  unset NEW_DB_PASSWORD
fi

TELEGRAM_BOT_TOKEN="$(get_env_var "TELEGRAM_BOT_TOKEN")"

# Helper function to seal a Kubernetes secret offline
# Arguments:
#   $1: namespace
#   $2: secret name
#   [--type=<type>]
#   [--no-namespace|--skip-namespace]
#   [--label=<key=val>]
#   $3..: secret key=value pairs
seal_secret() {
  local namespace="$1"
  local secret_name="$2"
  shift 2

  local secret_type="Opaque"
  local skip_namespace=false
  local labels=()

  while [[ "$#" -gt 0 ]]; do
    case "$1" in
      --type=*)
        secret_type="${1#--type=}"
        shift
        ;;
      --skip-namespace|--no-namespace)
        skip_namespace=true
        shift
        ;;
      --label=*)
        labels+=( "${1#--label=}" )
        shift
        ;;
      *)
        break
        ;;
    esac
  done

  local ns_dir="${PLATFORM_SECRETS_DIR}/${namespace}"
  mkdir -p "${ns_dir}"

  # Ensure namespace.yaml exists per namespace if not skipped and not already present
  if [ "$skip_namespace" = false ] && [ ! -f "${ns_dir}/namespace.yaml" ]; then
    local ns_file="${ns_dir}/namespace.yaml"
    cat <<EOF > "${ns_file}"
apiVersion: v1
kind: Namespace
metadata:
  name: ${namespace}
EOF
  fi

  local from_literal_args=()
  for item in "$@"; do
    local secret_key="${item%%=*}"
    local secret_val="${item#*=}"
    from_literal_args+=( "--from-literal=${secret_key}=${secret_val}" )
  done

  local sealed_file="${ns_dir}/${secret_name}.sealed.yaml"

  # Idempotency check: if sealed file exists and can be decrypted with recovery key,
  # avoid regenerating ciphertext if secret data is unchanged.
  local should_seal=true
  if [ -f "${sealed_file}" ] && [ -f "${SECRETS_DIR}/sealed-secrets.key" ]; then
    local desired_json current_json
    if [ "${#labels[@]}" -gt 0 ]; then
      desired_json="$(kubectl create secret generic "${secret_name}" \
        --namespace "${namespace}" \
        --type="${secret_type}" \
        "${from_literal_args[@]}" \
        --dry-run=client -o yaml | \
        kubectl label --local -f - "${labels[@]}" -o json 2>/dev/null || true)"
    else
      desired_json="$(kubectl create secret generic "${secret_name}" \
        --namespace "${namespace}" \
        --type="${secret_type}" \
        "${from_literal_args[@]}" \
        --dry-run=client -o json 2>/dev/null || true)"
    fi
    current_json="$(kubeseal --recovery-unseal --recovery-private-key "${SECRETS_DIR}/sealed-secrets.key" -f "${sealed_file}" -o json 2>/dev/null || true)"
    if [ -n "$desired_json" ] && [ -n "$current_json" ]; then
      if python3 -c '
import sys, json
d1 = json.loads(sys.argv[1]).get("data", {})
d2 = json.loads(sys.argv[2]).get("data", {})
sys.exit(0 if d1 == d2 else 1)
' "$desired_json" "$current_json" 2>/dev/null; then
        should_seal=false
      fi
    fi
  fi

  if [ "$should_seal" = true ]; then
    if [ "${#labels[@]}" -gt 0 ]; then
      kubectl create secret generic "${secret_name}" \
        --namespace "${namespace}" \
        --type="${secret_type}" \
        "${from_literal_args[@]}" \
        --dry-run=client -o yaml | \
      kubectl label --local -f - "${labels[@]}" -o yaml | \
      kubeseal \
        --cert "${CERT_FILE}" \
        --format yaml \
        --controller-name sealed-secrets-controller \
        --controller-namespace kube-system > "${sealed_file}"
    else
      kubectl create secret generic "${secret_name}" \
        --namespace "${namespace}" \
        --type="${secret_type}" \
        "${from_literal_args[@]}" \
        --dry-run=client -o yaml | \
      kubeseal \
        --cert "${CERT_FILE}" \
        --format yaml \
        --controller-name sealed-secrets-controller \
        --controller-namespace kube-system > "${sealed_file}"
    fi
    echo "Generated sealed secret: deploy/platform/secrets/${namespace}/${secret_name}.sealed.yaml"
  else
    echo "Sealed secret deploy/platform/secrets/${namespace}/${secret_name}.sealed.yaml is up to date (unchanged)."
  fi
}

# ------------------------------------------------------------------------------
# Secrets Table
# Add new secrets here as: seal_secret <namespace> <secret_name> [--type=...] <key1=val1> ...
# ------------------------------------------------------------------------------

# 1. Grafana admin secret (namespace: monitoring, name: grafana-admin)
seal_secret "monitoring" "grafana-admin" \
  "admin-user=admin" \
  "admin-password=${GRAFANA_ADMIN_PASSWORD}"

# 2. Notifier database secret (namespace: notifier, name: notifier-db-credentials)
seal_secret "notifier" "notifier-db-credentials" \
  --type="kubernetes.io/basic-auth" \
  "username=notifier" \
  "password=${NOTIFIER_DB_PASSWORD}"

# 3. Telegram secrets (Alertmanager & Argo Rollouts)
if [ -n "$TELEGRAM_BOT_TOKEN" ]; then
  seal_secret "monitoring" "alertmanager-telegram" \
    "bot_token=${TELEGRAM_BOT_TOKEN}"

  seal_secret "argo-rollouts" "argo-rollouts-notification-secret" \
    --no-namespace \
    --label="app.kubernetes.io/component=rollouts-controller" \
    --label="app.kubernetes.io/part-of=argo-rollouts" \
    "telegram-token=${TELEGRAM_BOT_TOKEN}"
else
  echo "Notice: TELEGRAM_BOT_TOKEN is not set in .env; skipping alertmanager-telegram and argo-rollouts-notification-secret"
fi

echo "All secrets successfully sealed offline."
