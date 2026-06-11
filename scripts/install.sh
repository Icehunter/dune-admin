#!/usr/bin/env bash
# install.sh — install the dune-admin binary on an Ubuntu host
#
# This script ONLY installs the binary. It does not configure it. To configure
# dune-admin after installing, run the built-in setup wizard:
#
#     /opt/dune-admin/dune-admin -setup
#
# The wizard asks you to pick a control plane (kubectl / docker / local / amp)
# and writes ~/.dune-admin/config.yaml.
#
# Prerequisites (this script does NOT install these for you):
#   - Ubuntu 22.04 or 24.04 with passwordless sudo
#   - Whatever your control plane needs already running:
#       * amp     → AMP installed with a running Dune instance
#                   (verify with: sudo -u amp ampinstmgr -l)
#       * kubectl → kubeconfig pointing at a k3s/k8s cluster with the
#                   dune workloads deployed
#       * docker  → docker daemon + named containers for dune services
#       * local   → game-server processes reachable on localhost
#   - PostgreSQL reachable (typically 127.0.0.1:15432 with the AMP module)
#   - Outbound internet (to fetch Go, Node, pnpm, the source repo)
#
# What this script does:
#   1. Installs build toolchain: Go 1.26.3, Node 22 LTS, pnpm 10.28.1, build-essential
#   2. Clones dune-admin source into $SOURCE_DIR (default: ~/src/dune-admin)
#   3. Builds the Linux binary + frontend assets
#   4. Copies the binary, SPA, and data files into $INSTALL_DIR (default: /opt/dune-admin)
#   5. Writes the systemd unit (Restart=always) — but does not enable/start it
#   6. Prints next steps: setup wizard, sudoers entry, service enable/start
#
# What this script does NOT do:
#   - Install AMP, k3s, Docker, or set up game services — those are prerequisites
#   - Run the setup wizard (interactive; you do that, see above)
#   - Apply sudoers grants (security-sensitive; you review and apply)
#   - Enable or start the systemd unit (it writes the unit, but you enable/start
#     it after running the setup wizard)
#
# Re-running this script is safe and idempotent. If a toolchain version is
# already correct, it's skipped. If source is already cloned, it's fetched
# and reset to the target branch. If artifacts already exist in $INSTALL_DIR,
# they are replaced atomically with a `.prev` backup left in place.
#
# Local patches:
#   If a "patches" directory exists next to this script (or one is specified
#   with --patches-dir), every *.patch file in it is applied with `git apply`
#   after the source checkout. Use this to layer in unmerged fixes or local
#   modifications without forking the repo. Pass --no-patches to skip.
#
# Usage:
#   ./install.sh                          # main branch, /opt/dune-admin, current user
#   ./install.sh --branch chore/phase-0-bug-fixes
#   ./install.sh --install-dir /opt/dune-admin --service-user dune-admin
#   ./install.sh --patches-dir ./my-patches
#   ./install.sh --no-patches
#   ./install.sh --help

# Re-exec under bash when started by another shell (`sh install.sh`, `sh -c …`,
# `zsh install.sh`) so the bash-only features below — arrays, [[ … ]],
# `set -o pipefail`, ${BASH_SOURCE} — work regardless of the caller's shell (#76).
# POSIX-safe test, and placed after the comment header so it stays out of the
# `sed -n '2,30p'` range that usage() prints. The README's `curl … | bash`
# already runs under bash, so the guard is a no-op there.
if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi

set -euo pipefail

# ── Defaults (override via flags) ─────────────────────────────────────────────
REPO_URL="https://github.com/Icehunter/dune-admin.git"
BRANCH="main"
SOURCE_DIR="${HOME}/src/dune-admin"
INSTALL_DIR="/opt/dune-admin"
SERVICE_USER="${USER}"
SKIP_TOOLCHAIN=0

# Patches directory: defaults to ./patches alongside this script. Empty/missing
# is fine — just means no patches will be applied.
# BASH_SOURCE[0] is unset when the script is piped via `curl | bash` (no file
# on disk), so guard with :- to avoid an unbound-variable error under set -u.
if [[ -n "${BASH_SOURCE[0]:-}" ]]; then
  SCRIPT_DIR="$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
else
  SCRIPT_DIR=""
fi
PATCHES_DIR="${SCRIPT_DIR:+${SCRIPT_DIR}/patches}"
APPLY_PATCHES=1

GO_VERSION="1.26.3"
NODE_MAJOR="22"
PNPM_VERSION="10.28.1"

# ── Helpers ──────────────────────────────────────────────────────────────────
log()  { printf '\033[1;34m[install]\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m[ ok   ]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[warn ]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[fail ]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
  sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
  exit 0
}

# ── Argument parsing ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)         REPO_URL="$2"; shift 2 ;;
    --branch)       BRANCH="$2"; shift 2 ;;
    --source-dir)   SOURCE_DIR="$2"; shift 2 ;;
    --install-dir)  INSTALL_DIR="$2"; shift 2 ;;
    --service-user) SERVICE_USER="$2"; shift 2 ;;
    --skip-toolchain) SKIP_TOOLCHAIN=1; shift ;;
    --patches-dir)  PATCHES_DIR="$2"; shift 2 ;;
    --no-patches)   APPLY_PATCHES=0; shift ;;
    -h|--help)      usage ;;
    *)              die "unknown flag: $1 (try --help)" ;;
  esac
done

log "config:"
log "  repo:          $REPO_URL"
log "  branch:        $BRANCH"
log "  source dir:    $SOURCE_DIR"
log "  install dir:   $INSTALL_DIR"
log "  service user:  $SERVICE_USER"
log ""

# ── Sanity checks ────────────────────────────────────────────────────────────
[[ "$(id -u)" -eq 0 ]] && die "run this as a normal user with sudo, not as root directly"
sudo -n true 2>/dev/null || die "this user needs passwordless sudo (or you need to authenticate sudo first)"
id "$SERVICE_USER" >/dev/null 2>&1 || die "service user '$SERVICE_USER' does not exist"
command -v sudo >/dev/null || die "sudo is required"

# Don't try to migrate a running service silently — make the operator stop it first.
if systemctl is-active --quiet dune-admin 2>/dev/null; then
  warn "dune-admin.service is currently active. Stop it before re-running this script:"
  warn "  sudo systemctl stop dune-admin"
  die  "refusing to swap binary under a running service"
fi

# ── 1. Toolchain ─────────────────────────────────────────────────────────────
if [[ "$SKIP_TOOLCHAIN" -eq 0 ]]; then
  log "installing build toolchain (apt: build-essential, git, curl, ca-certificates)…"
  sudo DEBIAN_FRONTEND=noninteractive apt-get update -qq
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
    build-essential git curl ca-certificates

  # ── Go ─────────────────────────────────────────────────────────────────────
  if /usr/local/go/bin/go version 2>/dev/null | grep -q "go${GO_VERSION}"; then
    ok "go ${GO_VERSION} already installed"
  else
    log "installing go ${GO_VERSION} to /usr/local/go…"
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o "$tmp/go.tgz"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "$tmp/go.tgz"
    ok "go $(/usr/local/go/bin/go version | awk '{print $3}') installed"
  fi
  export PATH="/usr/local/go/bin:${PATH}"

  # ── Node.js ────────────────────────────────────────────────────────────────
  if command -v node >/dev/null && node -v | grep -q "^v${NODE_MAJOR}\."; then
    ok "node $(node -v) already installed"
  else
    log "installing Node ${NODE_MAJOR} LTS from NodeSource…"
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | sudo -E bash -
    sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nodejs
    ok "node $(node -v) installed"
  fi

  # ── pnpm ───────────────────────────────────────────────────────────────────
  if command -v pnpm >/dev/null && pnpm -v 2>/dev/null | grep -q "^${PNPM_VERSION}$"; then
    ok "pnpm ${PNPM_VERSION} already installed"
  else
    log "installing pnpm ${PNPM_VERSION}…"
    sudo npm install -g "pnpm@${PNPM_VERSION}"
    ok "pnpm $(pnpm -v) installed"
  fi
else
  log "skipping toolchain installation (--skip-toolchain)"
  export PATH="/usr/local/go/bin:${PATH}"
fi
log ""

# ── 2. Source ────────────────────────────────────────────────────────────────
log "syncing source into $SOURCE_DIR @ branch $BRANCH…"
mkdir -p "$(dirname "$SOURCE_DIR")"
if [[ -d "$SOURCE_DIR/.git" ]]; then
  git -C "$SOURCE_DIR" fetch --quiet origin
else
  git clone --quiet "$REPO_URL" "$SOURCE_DIR"
fi
git -C "$SOURCE_DIR" checkout --quiet "$BRANCH" 2>/dev/null \
  || git -C "$SOURCE_DIR" checkout --quiet -B "$BRANCH" "origin/$BRANCH"
git -C "$SOURCE_DIR" reset --hard --quiet "origin/$BRANCH"
ok "checked out $(git -C "$SOURCE_DIR" rev-parse --short HEAD) ($(git -C "$SOURCE_DIR" log -1 --format=%s))"
log ""

# ── 2b. Local patches ────────────────────────────────────────────────────────
# Apply *.patch files (in lexical order) from $PATCHES_DIR. This lets you
# layer in unmerged fixes (e.g. our spaHandler restoration while the upstream
# PR is in review) without maintaining a fork.
if [[ "$APPLY_PATCHES" -eq 1 && -d "$PATCHES_DIR" ]]; then
  shopt -s nullglob
  patches=( "$PATCHES_DIR"/*.patch )
  shopt -u nullglob
  if [[ ${#patches[@]} -gt 0 ]]; then
    log "applying ${#patches[@]} local patch(es) from $PATCHES_DIR…"
    for p in "${patches[@]}"; do
      if git -C "$SOURCE_DIR" apply --check "$p" 2>/dev/null; then
        git -C "$SOURCE_DIR" apply "$p"
        ok "  applied $(basename "$p")"
      else
        # Already applied? Skip silently if reverse-apply works (idempotent re-runs).
        if git -C "$SOURCE_DIR" apply --reverse --check "$p" 2>/dev/null; then
          ok "  $(basename "$p") already applied (skipped)"
        else
          die "  $(basename "$p") does not apply cleanly. Inspect the patch and the target file:\n         git -C $SOURCE_DIR apply --check $p"
        fi
      fi
    done
  else
    log "no *.patch files in $PATCHES_DIR (skipping)"
  fi
elif [[ "$APPLY_PATCHES" -eq 0 ]]; then
  log "patch application disabled (--no-patches)"
else
  log "no patches dir at $PATCHES_DIR (skipping)"
fi
log ""

# ── 3. Build ─────────────────────────────────────────────────────────────────
log "building frontend (pnpm)…"
# Workaround for pnpm + rolldown native bindings on Linux/Windows: hoist
# node_modules so @rolldown/binding-* is resolvable from the top level.
echo 'node-linker=hoisted' > "$SOURCE_DIR/web/.npmrc"
(cd "$SOURCE_DIR/web" && pnpm install --frozen-lockfile && pnpm build) >/dev/null
[[ -f "$SOURCE_DIR/web/dist/index.html" ]] || die "frontend build produced no dist/index.html"
ok "frontend built ($(du -sh "$SOURCE_DIR/web/dist" | awk '{print $1}'))"

log "building backend (go)…"
(cd "$SOURCE_DIR" && make linux) >/dev/null
[[ -f "$SOURCE_DIR/dune-admin-linux" ]] || die "backend build produced no dune-admin-linux"
ok "backend built ($(du -sh "$SOURCE_DIR/dune-admin-linux" | awk '{print $1}'))"
log ""

# ── 4. Install into $INSTALL_DIR ─────────────────────────────────────────────
log "installing into $INSTALL_DIR (as service user '$SERVICE_USER')…"
sudo mkdir -p "$INSTALL_DIR"
sudo chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"

# Backup existing binary (move to .prev for one-step rollback).
if [[ -f "$INSTALL_DIR/dune-admin" ]]; then
  sudo cp -f "$INSTALL_DIR/dune-admin" "$INSTALL_DIR/dune-admin.prev"
fi
sudo install -m 0755 -o "$SERVICE_USER" -g "$SERVICE_USER" \
  "$SOURCE_DIR/dune-admin-linux" "$INSTALL_DIR/dune-admin"

# Backup existing dist (move to dist.prev for one-step rollback).
if [[ -d "$INSTALL_DIR/dist" ]]; then
  sudo rm -rf "$INSTALL_DIR/dist.prev"
  sudo mv "$INSTALL_DIR/dist" "$INSTALL_DIR/dist.prev"
fi
sudo cp -r "$SOURCE_DIR/web/dist" "$INSTALL_DIR/dist"
sudo chown -R "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR/dist"

# Data files (only copy if newer or missing — these change less often).
for f in item-data.json quality-data.json tags-data.json; do
  if [[ -f "$SOURCE_DIR/$f" ]]; then
    sudo install -m 0644 -o "$SERVICE_USER" -g "$SERVICE_USER" \
      "$SOURCE_DIR/$f" "$INSTALL_DIR/$f"
  fi
done
ok "installed: $(ls -la "$INSTALL_DIR/dune-admin" | awk '{print $NF, $5, "bytes"}')"
log ""

# ── 4b. systemd unit ─────────────────────────────────────────────────────────
# Write (or repair) the unit with Restart=always. This is REQUIRED for in-app
# self-update: after swapping the binary the process re-execs/exits, and only
# Restart=always reliably brings it back on a clean exit (a hand-made unit with
# Restart=on-failure leaves the service down after an update). We deliberately
# do NOT enable/start here — the service needs config.yaml from the setup
# wizard first — but we DO restart it if it is already enabled (re-install).
UNIT_PATH="/etc/systemd/system/dune-admin.service"
log "writing systemd unit $UNIT_PATH (Restart=always)…"
sudo tee "$UNIT_PATH" >/dev/null <<UNIT
[Unit]
Description=Dune Admin
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_USER
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/dune-admin
Restart=always
RestartSec=5s

[Install]
WantedBy=multi-user.target
UNIT
sudo systemctl daemon-reload
if systemctl is-enabled --quiet dune-admin 2>/dev/null; then
  log "existing service detected — restarting onto the new binary…"
  sudo systemctl restart dune-admin || warn "restart failed; check: sudo journalctl -u dune-admin -e"
fi
ok "systemd unit installed (Restart=always)"
log ""

# ── 5. Next steps ────────────────────────────────────────────────────────────
cat <<EOF

──────────────────────────────────────────────────────────────────────────────
 install complete. dune-admin binary + SPA are in $INSTALL_DIR.

 NEXT STEPS (each is intentionally manual so you can review):

 1) RUN THE SETUP WIZARD to generate ~/.dune-admin/config.yaml

      cd $INSTALL_DIR
      ./dune-admin -setup

    Select 'amp' as the control plane. Have these handy:
      - AMP instance name (e.g. DuneAwakening01) — run \`sudo -u amp ampinstmgr -l\`
      - OS user that runs AMP (typically 'amp')
      - PostgreSQL password (AMP module sets this during instance creation)

 2) APPLY SUDOERS GRANTS — the wizard prints an example at the end.
    Save it to /etc/sudoers.d/dune-admin and validate:

      sudo visudo -c

    Without this, the Server Settings tab cannot write the INI files.

 3) START THE SERVICE — the systemd unit is already installed at
    /etc/systemd/system/dune-admin.service (Restart=always, User=$SERVICE_USER).
    After the setup wizard has written the config, enable and start it:

      sudo systemctl enable --now dune-admin
      sudo journalctl -u dune-admin -f       # tail logs

    Browse to http://<this-host>:9090 (or whatever listen_addr you chose).

    NOTE: this installer writes the unit with Restart=always, which is required
    for in-app self-update (Settings → Check for Updates) to restart cleanly.

 ROLLBACK (if something is wrong):

      sudo systemctl stop dune-admin
      sudo cp $INSTALL_DIR/dune-admin.prev $INSTALL_DIR/dune-admin
      sudo rm -rf $INSTALL_DIR/dist && sudo mv $INSTALL_DIR/dist.prev $INSTALL_DIR/dist
      sudo systemctl start dune-admin

──────────────────────────────────────────────────────────────────────────────
EOF
