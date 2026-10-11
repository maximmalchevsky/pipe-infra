#!/usr/bin/env bash
# Ansible dynamic inventory script backed by Terraform outputs.

set -euo pipefail

TF_DIR="${TF_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../../terraform" && pwd)}"

if [[ "${1:-}" == "--list" ]]; then
  exec terraform -chdir="$TF_DIR" output -json ansible_inventory
elif [[ "${1:-}" == "--host" ]]; then
  echo '{}'
else
  echo "Usage: $0 --list | --host <hostname>" >&2
  exit 1
fi
