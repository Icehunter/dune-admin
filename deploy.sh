#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$ROOT_DIR"

VM_USER="${VM_USER:-dune}"
VM_HOST="${VM_HOST:-192.168.0.72}"
SSH_KEY_PATH="${SSH_KEY_PATH:-}"
KUBECONFIG_PATH="${KUBECONFIG_PATH:-$HOME/.kube/dune-external.yaml}"
IMAGE="${IMAGE:-}"
NAMESPACE="${NAMESPACE:-dune-admin}"
MANIFEST="${MANIFEST:-deploy/k8s/dune-admin.rendered.yaml}"
SKIP_KUBECONFIG=0
SKIP_BUILD=0
SKIP_IMAGE_IMPORT=0
NO_PORT_FORWARD=1

usage() {
  cat <<'EOF'
Usage: ./deploy.sh [options]

Options:
  --vm-user <user>           VM SSH user (default: dune)
  --vm-host <host>           VM host/IP (default: 192.168.0.72)
  --ssh-key <path>           SSH key path (default: ./sshKey when present)
  --kubeconfig <path>        Local kubeconfig path (default: ~/.kube/dune-external.yaml)
  --image <name:tag>         Image tag to build/deploy
                              default: dune-admin:local-<timestamp>
                              when skipping build/import: dune-admin:local
  --namespace <ns>           K8s namespace (default: dune-admin)
  --manifest <path>          Rendered manifest path (default: deploy/k8s/dune-admin.rendered.yaml)
  --skip-kubeconfig          Reuse existing kubeconfig without pulling from VM
  --skip-build               Skip docker build step
  --skip-image-import        Skip VM k3s image import step
  --port-forward             Open kubectl port-forward after deploy
  --no-port-forward          Skip kubectl port-forward after deploy (default)
  -h, --help                 Show this help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --vm-user) VM_USER="$2"; shift 2 ;;
    --vm-host) VM_HOST="$2"; shift 2 ;;
    --ssh-key) SSH_KEY_PATH="$2"; shift 2 ;;
    --kubeconfig) KUBECONFIG_PATH="$2"; shift 2 ;;
    --image) IMAGE="$2"; shift 2 ;;
    --namespace) NAMESPACE="$2"; shift 2 ;;
    --manifest) MANIFEST="$2"; shift 2 ;;
    --skip-kubeconfig) SKIP_KUBECONFIG=1; shift ;;
    --skip-build) SKIP_BUILD=1; shift ;;
    --skip-image-import) SKIP_IMAGE_IMPORT=1; shift ;;
    --port-forward) NO_PORT_FORWARD=0; shift ;;
    --no-port-forward) NO_PORT_FORWARD=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

require_cmd kubectl
require_cmd ssh
require_cmd scp
require_cmd docker
require_cmd make

if [[ -z "$SSH_KEY_PATH" && -f "$ROOT_DIR/sshKey" ]]; then
  SSH_KEY_PATH="$ROOT_DIR/sshKey"
fi
if [[ -z "$IMAGE" ]]; then
  if [[ "$SKIP_BUILD" -eq 1 || "$SKIP_IMAGE_IMPORT" -eq 1 ]]; then
    IMAGE="dune-admin:local"
  else
    IMAGE="dune-admin:local-$(date +%Y%m%d%H%M%S)"
  fi
fi

SSH_OPTS=(-o PreferredAuthentications=publickey,password -o PubkeyAuthentication=yes -o PasswordAuthentication=yes)
SCP_OPTS=(-o PreferredAuthentications=publickey,password -o PubkeyAuthentication=yes -o PasswordAuthentication=yes)
if [[ -n "$SSH_KEY_PATH" ]]; then
  if [[ ! -f "$SSH_KEY_PATH" ]]; then
    echo "SSH key not found: $SSH_KEY_PATH" >&2
    exit 1
  fi
  SSH_OPTS+=(-i "$SSH_KEY_PATH" -o IdentitiesOnly=yes)
  SCP_OPTS+=(-i "$SSH_KEY_PATH" -o IdentitiesOnly=yes)
  echo "Using SSH key: $SSH_KEY_PATH (fallback to password enabled)"
else
  echo "No SSH key provided/found; using password auth (or agent) for SSH."
fi

if [[ "$SKIP_KUBECONFIG" -eq 0 ]]; then
  mkdir -p "$(dirname "$KUBECONFIG_PATH")"
  echo "Pulling kubeconfig from ${VM_USER}@${VM_HOST}..."
  ssh "${SSH_OPTS[@]}" "${VM_USER}@${VM_HOST}" "sudo cat /etc/rancher/k3s/k3s.yaml" > "$KUBECONFIG_PATH"
  awk -v host="$VM_HOST" '{gsub(/127\.0\.0\.1/, host); print}' "$KUBECONFIG_PATH" > "${KUBECONFIG_PATH}.tmp"
  mv "${KUBECONFIG_PATH}.tmp" "$KUBECONFIG_PATH"
fi

export KUBECONFIG="$KUBECONFIG_PATH"
echo "Using KUBECONFIG=$KUBECONFIG"
kubectl get nodes

if [[ "$SKIP_BUILD" -eq 0 ]]; then
  echo "Building image: $IMAGE"
  docker buildx build --platform linux/amd64 -f deploy/Dockerfile -t "$IMAGE" --load .
fi

if [[ "$SKIP_IMAGE_IMPORT" -eq 0 ]]; then
  echo "Importing image into k3s runtime on ${VM_HOST}..."
  tmp_tar="$(mktemp /tmp/dune-admin-image.XXXXXX.tar)"
  trap 'rm -f "$tmp_tar"' EXIT
  docker save -o "$tmp_tar" "$IMAGE"
  scp "${SCP_OPTS[@]}" "$tmp_tar" "${VM_USER}@${VM_HOST}:/tmp/dune-admin-image.tar"
  ssh "${SSH_OPTS[@]}" "${VM_USER}@${VM_HOST}" "sudo k3s ctr images import /tmp/dune-admin-image.tar && rm -f /tmp/dune-admin-image.tar"
  rm -f "$tmp_tar"
  trap - EXIT
fi

echo "Rendering k8s manifest..."
make render-k8s

if [[ ! -f "$MANIFEST" ]]; then
  echo "Manifest not found: $MANIFEST" >&2
  exit 1
fi

control_ns="$(awk -F':[[:space:]]*' '/^[[:space:]]*control_namespace:[[:space:]]*/{gsub(/"/,"",$2); print $2; exit}' "$MANIFEST")"
db_host_override=""
if [[ -n "$control_ns" ]]; then
  svc_rows="$(kubectl -n "$control_ns" get svc -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.ports[*]}{.port}{" "}{end}{"\n"}{end}' 2>/dev/null || true)"
  db_svc="$(printf '%s\n' "$svc_rows" | awk '$0 ~ /(^|[[:space:]])15432([[:space:]]|$)/{print $1; exit}')"
  if [[ -n "$db_svc" ]]; then
    db_host_override="${db_svc}.${control_ns}.svc.cluster.local"
    echo "Using in-cluster DB host: $db_host_override"
  fi
fi

awk -v image="$IMAGE" -v dbhost="$db_host_override" '
  BEGIN { image_changed = 0 }
  /^[[:space:]]*image:[[:space:]]*/ && image_changed == 0 {
    sub(/image:.*/, "image: " image)
    image_changed = 1
  }
  /^[[:space:]]*CONTROL:[[:space:]]*/ {
    sub(/CONTROL:.*/, "CONTROL: \"local\"")
  }
  /^[[:space:]]*control:[[:space:]]*/ {
    sub(/control:.*/, "control: local")
  }
  /^[[:space:]]*cmd_status:[[:space:]]*/ { next }
  /^[[:space:]]*cmd_start:[[:space:]]*/ { next }
  /^[[:space:]]*cmd_stop:[[:space:]]*/ { next }
  /^[[:space:]]*cmd_restart:[[:space:]]*/ { next }
  dbhost != "" && /^[[:space:]]*DB_HOST:[[:space:]]*/ {
    sub(/DB_HOST:.*/, "DB_HOST: \"" dbhost "\"")
  }
  dbhost != "" && /^[[:space:]]*db_host:[[:space:]]*/ {
    sub(/db_host:.*/, "db_host: " dbhost)
  }
  /^[[:space:]]*ssh_host:[[:space:]]*/ { next }
  /^[[:space:]]*ssh_user:[[:space:]]*/ { next }
  /^[[:space:]]*ssh_key:[[:space:]]*/ { next }
  /^[[:space:]]*MARKET_BOT_ENABLED:[[:space:]]*/ {
    sub(/MARKET_BOT_ENABLED:.*/, "MARKET_BOT_ENABLED: \"true\"")
  }
  /^[[:space:]]*market_bot_enabled:[[:space:]]*/ {
    sub(/market_bot_enabled:.*/, "market_bot_enabled: true")
  }
  /^[[:space:]]*market_bot_item_data:[[:space:]]*/ {
    sub(/market_bot_item_data:.*/, "market_bot_item_data: /app/item-data.json")
  }
  /^[[:space:]]*market_bot_cache_db:[[:space:]]*/ {
    sub(/market_bot_cache_db:.*/, "market_bot_cache_db: /data/market-bot-cache.db")
  }
  { print }
  END {
    if (image_changed == 0) {
      print "No image: field found to patch in manifest" > "/dev/stderr"
      exit 1
    }
  }
' "$MANIFEST" > "${MANIFEST}.tmp"
mv "${MANIFEST}.tmp" "$MANIFEST"

echo "Applying manifest..."
kubectl apply -f "$MANIFEST"
if [[ -n "$control_ns" ]]; then
  echo "Applying RBAC for in-cluster control access..."
  cat <<EOF | kubectl apply -f -
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: dune-admin-runtime
  namespace: $control_ns
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log", "services", "endpoints", "persistentvolumeclaims"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create", "get"]
  - apiGroups: ["igw.funcom.com"]
    resources: ["battlegroups", "serverstats"]
    verbs: ["get", "list", "watch", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: dune-admin-runtime
  namespace: $control_ns
subjects:
  - kind: ServiceAccount
    name: default
    namespace: $NAMESPACE
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: dune-admin-runtime
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: dune-admin-operators-logs
  namespace: funcom-operators
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: dune-admin-operators-logs
  namespace: funcom-operators
subjects:
  - kind: ServiceAccount
    name: default
    namespace: $NAMESPACE
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: dune-admin-operators-logs
EOF
fi
kubectl -n "$NAMESPACE" rollout restart deploy/dune-admin
kubectl -n "$NAMESPACE" rollout status deploy/dune-admin
kubectl -n "$NAMESPACE" get pods,svc

echo "Running in-cluster health checks (fast-fail, no pod waits)..."
stale_pods="$(kubectl -n "$NAMESPACE" get pods --no-headers 2>/dev/null | awk '/^curl($|-check-)/{print $1}')"
if [[ -n "$stale_pods" ]]; then
  while IFS= read -r p; do
    [[ -n "$p" ]] && kubectl -n "$NAMESPACE" delete pod "$p" --ignore-not-found >/dev/null 2>&1 || true
  done <<< "$stale_pods"
fi
status_path="/api/v1/namespaces/$NAMESPACE/services/http:dune-admin:8080/proxy/api/v1/status"
bot_path="/api/v1/namespaces/$NAMESPACE/services/http:dune-admin:8080/proxy/api/v1/market-bot/status"
bg_path="/api/v1/namespaces/$NAMESPACE/services/http:dune-admin:8080/proxy/api/v1/battlegroup/status"
health_ok=0
last_status=""
last_bot=""
last_bg=""
for i in $(seq 1 30); do
  last_status="$(kubectl --request-timeout=5s get --raw "$status_path" 2>/dev/null || true)"
  last_bot="$(kubectl --request-timeout=5s get --raw "$bot_path" 2>/dev/null || true)"
  last_bg="$(kubectl --request-timeout=5s get --raw "$bg_path" 2>/dev/null || true)"
  if [[ -n "$last_status" ]] &&
     echo "$last_bot" | grep -q "\"enabled\":true" &&
     ! echo "$last_bg" | grep -q "does not support GetStatus"; then
    echo "$last_bot"
    health_ok=1
    break
  fi
  if (( i % 5 == 0 )); then
    echo "Health check retry $i/30..."
  fi
  sleep 1
done
if [[ "$health_ok" -ne 1 ]]; then
  echo "health check failed: API or embedded market-bot not ready" >&2
  echo "last /api/v1/status: $last_status" >&2
  echo "last /api/v1/market-bot/status: $last_bot" >&2
  echo "last /api/v1/battlegroup/status: $last_bg" >&2
  exit 1
fi

echo "Deploy complete (image: $IMAGE)."

if [[ "$NO_PORT_FORWARD" -eq 0 ]]; then
  echo "Opening API port-forward at http://127.0.0.1:8080 ..."
  kubectl -n "$NAMESPACE" port-forward svc/dune-admin 8080:8080
else
  echo "Deploy complete. Run ./listen.sh to open API port-forward."
fi
