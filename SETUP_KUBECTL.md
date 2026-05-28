# Provider: kubectl (Kubernetes / k3s)

Use this provider when your Dune server runs inside a Kubernetes cluster (e.g. k3s on a VM) and dune-admin runs on your local machine or another host with SSH access to that VM.

All commands run over SSH — no exposed ports, no VPN.

```
your machine
  └─ SSH tunnel → VM
       ├─ kubectl → battlegroup CRDs / pod logs
       └─ TCP tunnel → PostgreSQL (pod IP:15432)
```

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| **Go 1.21+** | `brew install go` or <https://go.dev/dl/> |
| **SSH key** | Private key authorised on the VM |
| **VM access** | Port 22 reachable; SSH user needs passwordless `sudo kubectl` |

## Quick start (wizard)

```bash
# Place SSH key (checked automatically in this order):
#   ./sshKey  │  ~/.dune-admin/sshKey  │  ~/.ssh/dune  │  ~/.ssh/id_ed25519  │  ~/.ssh/id_rsa
cp /path/to/key ./sshKey && chmod 600 ./sshKey

make setup   # prompts for VM host:port, discovers namespace + DB password automatically
make build   # builds frontend + dune-admin binary
./dune-admin
```

The wizard:

1. Locates your SSH key
2. SSHes into the VM
3. Runs `kubectl get pods -A` to find the database pod and namespace
4. Reads `~/.dune/<battlegroup>.yaml` on the VM for DB credentials
5. Writes `~/.dune-admin/config.yaml`

## External VM deploy example (`dune@192.168.0.72`)

```bash
cd /Volumes/Engineering/Icehunter/dune-admin

# Pull kubeconfig from VM and point kubectl at the external cluster.
mkdir -p ~/.kube
ssh dune@192.168.0.72 "sudo cat /etc/rancher/k3s/k3s.yaml" > ~/.kube/dune-external.yaml
sed -i '' 's/127.0.0.1/192.168.0.72/g' ~/.kube/dune-external.yaml
export KUBECONFIG=~/.kube/dune-external.yaml
kubectl get nodes

# Configure dune-admin runtime values.
make setup

# Ensure these are set in ~/.dune-admin/config.yaml for container runtime:
# market_bot_enabled: true
# market_bot_item_data: /app/item-data.json
# market_bot_cache_db: /data/market-bot-cache.db

# Build local image and import it into k3s runtime on the VM.
docker buildx build --platform linux/amd64 -f deploy/Dockerfile -t dune-admin:local --load .
docker save dune-admin:local | ssh dune@192.168.0.72 "sudo k3s ctr images import -"

# Render deployment manifest from ~/.dune-admin/config.yaml and switch image tag.
make render-k8s
sed -i '' 's#ghcr.io/icehunter/dune-admin:latest#dune-admin:local#g' deploy/k8s/dune-admin.rendered.yaml

# Deploy and wait for readiness.
kubectl apply -f deploy/k8s/dune-admin.rendered.yaml
kubectl -n dune-admin rollout status deploy/dune-admin
kubectl -n dune-admin get pods,svc

# Verify API and bot from inside the cluster.
kubectl -n dune-admin run curl --rm -it --restart=Never --image=curlimages/curl -- \
  sh -c "curl -s http://dune-admin:8080/api/v1/market-bot/status"

# Local access from your laptop.
kubectl -n dune-admin port-forward svc/dune-admin 8080:8080
```

## Scripted deploy (Linux/macOS + Windows)

From repo root:

```bash
./deploy.sh
```

```powershell
./deploy.ps1
```

> On Windows, if script execution is blocked, run once:
> `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`

Both scripts run the full k8s flow:

1. Pull kubeconfig from VM (`dune@192.168.0.72` by default) unless skipped
2. Build a fresh timestamped image tag (`dune-admin:local-<timestamp>` by default)
3. Import image into VM k3s runtime (`k3s ctr images import`)
4. Render and apply `deploy/k8s/dune-admin.rendered.yaml`
5. Auto-fix embedded bot deployment settings in the rendered manifest:
   - `MARKET_BOT_ENABLED: "true"`
   - `market_bot_enabled: true`
   - `market_bot_item_data: /app/item-data.json`
   - `market_bot_cache_db: /data/market-bot-cache.db`
6. Restart rollout, wait for old terminating pods to drain, then run in-cluster health checks for:
   - `/api/v1/status` reachable
   - `/` returns HTTP 200 (no UI 404)
   - `/api/v1/market-bot/status` reports `"enabled":true`
7. Open `kubectl port-forward` on `127.0.0.1:8080` (unless disabled)

SSH auth behavior in both scripts:

- If `./sshKey` exists, it is used first (`-i ./sshKey`)
- If key auth fails or no key is present, SSH falls back to password prompt

### Script options

| Purpose | Bash | PowerShell |
|---|---|---|
| VM user | `--vm-user dune` | `-VmUser dune` |
| VM host | `--vm-host 192.168.0.72` | `-VmHost 192.168.0.72` |
| SSH key path | `--ssh-key ./sshKey` | `-SshKeyPath .\sshKey` |
| Kubeconfig path | `--kubeconfig ~/.kube/dune-external.yaml` | `-KubeconfigPath "$HOME/.kube/dune-external.yaml"` |
| Image tag | `--image dune-admin:local` | `-Image dune-admin:local` |
| Namespace | `--namespace dune-admin` | `-Namespace dune-admin` |
| Manifest path | `--manifest deploy/k8s/dune-admin.rendered.yaml` | `-Manifest deploy/k8s/dune-admin.rendered.yaml` |
| Skip kubeconfig pull | `--skip-kubeconfig` | `-SkipKubeconfig` |
| Skip image build | `--skip-build` | `-SkipBuild` |
| Skip VM image import | `--skip-image-import` | `-SkipImageImport` |
| Skip port-forward | `--no-port-forward` | `-NoPortForward` |

### First deploy vs quick redeploy

First deploy:

```bash
./deploy.sh
```

Quick redeploy (reuse kubeconfig, no auto port-forward):

```bash
./deploy.sh --skip-kubeconfig --no-port-forward
```

Quick redeploy with the exact same image tag (advanced):

```bash
./deploy.sh --image dune-admin:local --skip-kubeconfig --no-port-forward
```

Override defaults with flags (same names on both scripts), e.g.:

```bash
./deploy.sh --vm-user dune --vm-host 192.168.0.72 --image dune-admin:local --no-port-forward
```

```powershell
./deploy.ps1 -VmUser dune -VmHost 192.168.0.72 -Image dune-admin:local -NoPortForward
```

## Manual config (`~/.dune-admin/config.yaml`)

```yaml
control: kubectl

ssh_host: 192.168.0.72:22   # VM host:port
ssh_user: dune               # SSH user
ssh_key: /home/you/.ssh/key  # absolute path; omit to use auto-detection

db_host: 127.0.0.1           # unused for kubectl — pod IP is discovered automatically
db_port: 15432
db_user: postgres
db_pass: yourpassword
db_name: dune
db_schema: dune

# Optional — discovered automatically if omitted:
control_namespace: funcom-seabass-mybattlegroup

# Optional broker command path:
broker_game_addr: 10.43.48.246:5672
broker_admin_addr: 10.43.189.193:5672
broker_tls: true

# Optional:
backup_dir: /funcom/artifacts/database-dumps/mybattlegroup
listen_addr: :8080
scrip_currency: 1
```

## What works

| Feature | Supported |
|---------|-----------|
| Battlegroup status (phase, servers) | Yes |
| Start / stop / restart | Yes — `kubectl patch battlegroup` |
| Update / backup | Yes — `battlegroup.sh` |
| Pod list | Yes |
| Log streaming | Yes — `kubectl logs -f` |
| DB access | Yes — tunnelled through SSH to pod IP |
| RabbitMQ broker commands | Yes — `kubectl exec` into broker pod |
| Backup download / upload / restore | Yes |

## Backward compatibility

Existing `.env` files with just `SSH_HOST`, `SSH_USER`, `DB_PASS`, etc. continue to work unchanged. The control plane defaults to `kubectl` whenever `SSH_HOST` is set, and the namespace is auto-discovered at startup.

## Troubleshooting

**Battlegroup tab shows nothing** — the namespace was not discovered. Check that the SSH user can run `sudo kubectl get pods -A`. You can also pin it explicitly with `control_namespace` in config.yaml.

**DB connection fails** — pod discovery succeeded but DB password is wrong. Delete `~/.dune-admin/config.yaml` and re-run `make setup`.

**"sudo: kubectl: command not found"** — kubectl is not in the sudo-safe PATH on the VM. Add `/usr/local/bin` (or wherever kubectl lives) to `/etc/sudoers` `secure_path`.

**Port-forward works but `http://127.0.0.1:8080` returns 404** — you are running an old image that does not contain the built frontend (`/app/dist`). Rebuild and redeploy with the deploy script.

**Market bot panel shows inactive after deploy** — verify:

1. `market_bot_enabled: true` in `~/.dune-admin/config.yaml`
2. `market_bot_item_data: /app/item-data.json`
3. `market_bot_cache_db: /data/market-bot-cache.db`

Then redeploy and check:

```bash
kubectl -n dune-admin logs deploy/dune-admin --tail=120
kubectl -n dune-admin run curl --rm -it --restart=Never --image=curlimages/curl -- \
  sh -c "curl -s http://dune-admin:8080/api/v1/market-bot/status"
```
