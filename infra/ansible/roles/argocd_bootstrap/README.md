# argocd_bootstrap

Bootstraps ArgoCD on a Kubernetes control-plane node using Helm. Installs Helm if missing or mismatched, registers the Argo Helm repo, deploys the `argo/argo-cd` chart in insecure mode, installs `python3-kubernetes`, and optionally applies an initial root Application (App-of-Apps) manifest.

## Why No Ingress Yet
Cluster networking and ingress controllers (e.g. MetalLB, ingress-nginx, cert-manager) are not yet provisioned. In accordance with GitOps principles, all networking and ingress components will be deployed declaratively by ArgoCD itself from Git. ArgoCD initially runs with `server.ingress.enabled: false` and is accessed via `kubectl port-forward`.

## Variables

| Variable | Default | Description |
|---|---|---|
| `helm_version` | `"v3.21.2"` | Helm binary version to install |
| `argocd_namespace` | `"argocd"` | Kubernetes namespace for ArgoCD |
| `argocd_chart_version` | `"10.1.2"` | `argo/argo-cd` Helm chart version |
| `argocd_kubeconfig` | `"/etc/kubernetes/admin.conf"` | Cluster admin kubeconfig path |
| `argocd_root_app_repo_url` | `""` | Git repo URL for root Application (empty skips) |
| `argocd_root_app_path` | `"deploy/root"` | Path in Git repo for root Application |
| `argocd_root_app_revision` | `"main"` | Git branch/tag/revision for root Application |
| `argocd_redis_image_repository` | `"docker.io/library/redis"` | Redis image repository (overrides ECR to bypass reachability issues) |
