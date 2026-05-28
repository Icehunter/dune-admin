#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

NAMESPACE="${NAMESPACE:-dune-admin}"
LOCAL_PORT="${LOCAL_PORT:-8080}"
REMOTE_PORT="${REMOTE_PORT:-8080}"

usage() {
  cat <<'EOF'
Usage: ./listen.sh [options]

Opens a kubectl port-forward to the dune-admin service.

NOTE: The service is now type=NodePort exposed on port 30080 on the VM,
so you can also reach it directly at http://<VM-IP>:30080 without this script.
This script is only needed if you want to map a specific local port.

Options:
  --namespace <ns>         Kubernetes namespace (default: dune-admin)
  --local-port <port>      Local listen port (default: 8080)
  --remote-port <port>     Service target port (default: 8080)
  -h, --help               Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --namespace) NAMESPACE="$2"; shift 2 ;;
    --local-port) LOCAL_PORT="$2"; shift 2 ;;
    --remote-port) REMOTE_PORT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

echo "Opening API port-forward at http://127.0.0.1:${LOCAL_PORT} ..."
echo "(Alternatively, use http://<VM-IP>:30080 directly via NodePort)"
kubectl -n "$NAMESPACE" port-forward svc/dune-admin "${LOCAL_PORT}:${REMOTE_PORT}"
