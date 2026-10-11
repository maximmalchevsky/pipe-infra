#!/usr/bin/env bash
# Generates a restricted-compliant Kubernetes Job manifest for notifier load generation.
# Outputs YAML to stdout with PSA restricted securityContext and NetworkPolicy labels.
set -euo pipefail

if [[ $# -lt 2 ]]; then
  echo "Usage: $0 <job-name> <image> [loadgen-args...]" >&2
  exit 1
fi

JOB_NAME="$1"
IMAGE="$2"
shift 2

cat <<EOF
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB_NAME}
  namespace: notifier
  labels:
    app.kubernetes.io/name: notifier-loadgen
    app.kubernetes.io/part-of: notifier
spec:
  backoffLimit: 0
  ttlSecondsAfterFinished: 600
  template:
    metadata:
      labels:
        app.kubernetes.io/name: notifier-loadgen
        app.kubernetes.io/part-of: notifier
    spec:
      restartPolicy: Never
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
        seccompProfile:
          type: RuntimeDefault
      containers:
        - name: loadgen
          image: "${IMAGE}"
          command:
            - "/usr/local/bin/loadgen"
EOF

for arg in "$@"; do
  escaped_arg=$(printf '%s' "$arg" | sed 's/\\/\\\\/g; s/"/\\"/g')
  echo "            - \"${escaped_arg}\""
done

cat <<EOF
          securityContext:
            runAsNonRoot: true
            runAsUser: 65532
            runAsGroup: 65532
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            capabilities:
              drop:
                - ALL
            seccompProfile:
              type: RuntimeDefault
          resources:
            requests:
              cpu: 100m
              memory: 32Mi
            limits:
              memory: 128Mi
EOF
