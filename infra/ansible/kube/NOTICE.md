# NOTICE

This directory remains under GPL-3.0; the rest of the repository is MIT-licensed.

This directory contains code adapted from [BigKAA/00-kube-ansible](https://github.com/BigKAA/00-kube-ansible).

- **Upstream URL**: https://github.com/BigKAA/00-kube-ansible
- **Commit**: `3f5ecfa3196301b0045a4fa0157830e66123c11e`
- **License**: GNU General Public License v3.0 (GPL-3.0)
- **Author**: BigKAA

## Modifications
- Vendored into `infra/ansible/kube/` excluding author-specific example inventories, container configs, and documentation.
- Moved `group_vars` into `infra/ansible/inventory/group_vars/`.
- Adjusted default variables in `group_vars/k8s_cluster` for hackathon pipeline (disabled unnecessary addons, updated IP ranges, cleared author-specific secrets and values).
- Fixed host preparation for Ubuntu 24.04: replaced `ntp` with `chrony` in `roles/prepare-hosts/vars/Debian.yaml`.
- Added Debian/Ubuntu package installation support for Keepalived and HAProxy in `roles/ha`.
- Adapted `roles/prepare-hosts/tasks/containerd.yaml` to support containerd 2.2+ idempotently while preserving 1.x compatibility, using a restart handler.
- Added Debian/Ubuntu package uninstallation tasks in `reset.yaml`.
- Added Debian repository update and package upgrade tasks in `roles/upgrade-cluster/tasks/_update-repo.yaml`.
- Sanitized author-specific URLs, IPs, and repository entries across role templates and documentation.
- Fixed stray quote in Docker apt repo line (upstream bug).
- Fixed containerd `hosts.toml` template for Docker Hub mirror (updated server endpoint to `registry-1.docker.io` and stripped redundant scheme and `/v2` path).
- Made `second_controls` role idempotent: gated on `groups['k8s_masters'] | length > 1`, checked `/etc/kubernetes/kubelet.conf` via stat, marked read-only lookups with `changed_when: false`, and conditioned CoreDNS restart only on actual node join.
- Fixed directory modes across roles to ensure execute bits (`0755` for `/etc/containerd`, certs.d, CNI, and container registry directories; quoted `'0700'` for private directories).
- Wrapped `prepare-hosts` hook inclusions in conditional blocks to prevent changed status on empty task lists.
