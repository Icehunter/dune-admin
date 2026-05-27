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
make build
./dune-admin
```

The wizard:

1. Locates your SSH key
2. SSHes into the VM
3. Runs `kubectl get pods -A` to find the database pod and namespace
4. Reads `~/.dune/<battlegroup>.yaml` on the VM for DB credentials
5. Writes `~/.dune-admin/config.yaml`

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

# Optional capture mode:
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
| RabbitMQ capture | Yes — `kubectl exec` into broker pod |
| Backup download / upload / restore | Yes |

## Backward compatibility

Existing `.env` files with just `SSH_HOST`, `SSH_USER`, `DB_PASS`, etc. continue to work unchanged. The control plane defaults to `kubectl` whenever `SSH_HOST` is set, and the namespace is auto-discovered at startup.

## Troubleshooting

**Battlegroup tab shows nothing** — the namespace was not discovered. Check that the SSH user can run `sudo kubectl get pods -A`. You can also pin it explicitly with `control_namespace` in config.yaml.

**DB connection fails** — pod discovery succeeded but DB password is wrong. Delete `~/.dune-admin/config.yaml` and re-run `make setup`.

**"sudo: kubectl: command not found"** — kubectl is not in the sudo-safe PATH on the VM. Add `/usr/local/bin` (or wherever kubectl lives) to `/etc/sudoers` `secure_path`.
