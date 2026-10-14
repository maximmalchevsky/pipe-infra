# Upstream Grafana Dashboards

| Файл | Upstream URL | Ревизия / Тег | Лицензия |
| --- | --- | --- | --- |
| `ingress-nginx.json` | https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.15.1/deploy/grafana/dashboards/nginx.json | `controller-v1.15.1` | Apache-2.0 |
| `ingress-nginx-request-handling.json` | https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.15.1/deploy/grafana/dashboards/request-handling-performance.json | `controller-v1.15.1` | Apache-2.0 |
| `argo-rollouts.json` | https://raw.githubusercontent.com/argoproj/argo-rollouts/v1.10.1/examples/dashboard.json | `v1.10.1` | Apache-2.0 |
| `cloudnative-pg.json` | https://grafana.com/api/dashboards/20417/revisions/4/download | revision 4 | Apache-2.0 |
| `go-processes.json` | https://grafana.com/api/dashboards/6671/revisions/2/download | revision 2 | не указана на grafana.com |
| `keda.json` | https://raw.githubusercontent.com/kedacore/keda/v2.21.0/config/grafana/keda-dashboard.json | `v2.21.0` | Apache-2.0 |
| `cert-manager.json` | https://grafana.com/api/dashboards/20842/revisions/3/download | revision 3 | не указана на grafana.com |
| `sealed-secrets.json` | https://raw.githubusercontent.com/bitnami-labs/sealed-secrets/v0.40.0/contrib/prometheus-mixin/dashboards/sealed-secrets-controller.json | `v0.40.0` | Apache-2.0 |
| `argocd.json` | https://grafana.com/api/dashboards/19993/revisions/9/download | revision 9 | не указана на grafana.com |

JSON не редактируется; плейсхолдеры ${DS_*} подменяются на uid prometheus при рендере
