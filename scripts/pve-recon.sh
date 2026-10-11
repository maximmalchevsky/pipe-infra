#!/usr/bin/env bash
# Read-only Proxmox reconnaissance: dumps cluster facts into tmp/pve-recon/*.json.
# Uses only GET requests via scripts/pve-get.sh; the API token never lands in the output.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/tmp/pve-recon"
GET="$ROOT/scripts/pve-get.sh"
mkdir -p "$OUT"

dump() { # dump <file> <api-path>
  if "$GET" "$2" > "$OUT/$1.json" 2> "$OUT/$1.err"; then
    rm -f "$OUT/$1.err"; echo "ok   $2"
  else
    echo "FAIL $2 (see $OUT/$1.err)"
  fi
}

dump version        /version
dump nodes          /nodes
dump storage        /storage
dump cluster-status /cluster/status
dump nextid         /cluster/nextid

nodes=$(python3 -c 'import json,sys; print(" ".join(n["node"] for n in json.load(sys.stdin)["data"]))' < "$OUT/nodes.json")
for n in $nodes; do
  dump "node-$n-status"  "/nodes/$n/status"
  dump "node-$n-storage" "/nodes/$n/storage"
  dump "node-$n-network" "/nodes/$n/network"
  dump "node-$n-qemu"    "/nodes/$n/qemu"
  dump "node-$n-lxc"     "/nodes/$n/lxc"
  dump "node-$n-dns"     "/nodes/$n/dns"
done

echo "done: $OUT"
