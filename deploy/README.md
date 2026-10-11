# GitOps Platform Layer

## Sync Waves
- Wave 0: Core controllers (cert-manager, metallb, local-path-provisioner)
- Wave 1: Configurations (metallb-config, cert-manager-config)
- Wave 2: Ingress controller (ingress-nginx with MetalLB loadBalancerIP)
- Wave 3: Ingress resources (argocd-ingress)

## Values Flow
`.env` -> Terraform outputs -> Ansible inventory -> root Application `helm.valuesObject` (repoURL, targetRevision, domain, metallbIP) -> sub-apps.

## Adding an Application
Add an ArgoCD `Application` manifest in `deploy/root/templates/` specifying wave, destination namespace, and `helm.valuesObject`.
