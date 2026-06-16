#!/usr/bin/env bash
# install.sh — install the dune-admin binary on a Linux host
#
# Downloads the latest (or a pinned) GitHub release. No build toolchain
# required — the release binary already embeds the frontend SPA.
#
# Prerequisites:
#   - Ubuntu 22.04 or 24.04 with passwordless sudo
#   - curl and tar (present on all Ubuntu installs)
#   - Your control-plane stack already running (AMP, docker, k3s, …)
#   - PostgreSQL reachable (typically 127.0.0.1:15432 with the AMP module)
#
# What this script does:
#   1. Detects OS / arch and resolves the matching release asset
#   2. Downloads + extracts the release tarball from GitHub
#   3. Copies the binary and data files into $INSTALL_DIR (default /opt/dune-admin)
#   4. Writes the systemd unit (Restart=always) — but does not enable/start it
#   5. Prints next steps: setup wizard, sudoers entry, service enable/start
#
# Re-running is safe and idempotent. The previous binary is kept as
# dune-admin.prev in $INSTALL_DIR for one-step rollback.
#
# Usage:
#   ./install.sh
#   ./install.sh --version v0.9.2
#   ./install.sh --install-dir /opt/dune-admin --service-user dune-admin
#   ./install.sh --help

# Re-exec under bash when started by another shell (sh, zsh, curl|sh, …).
# POSIX-safe guard; placed after the comment header so `sed -n '2,30p'`
# that usage() prints stays readable.
if [ -z "${BASH_VERSION:-}" ]; then
  exec bash "$0" "$@"
fi

set -euo pipefail

# ── Defaults (override via flags) ─────────────────────────────────────────────
REPO="Icehunter/dune-admin"
VERSION="latest"
INSTALL_DIR="/opt/dune-admin"
SERVICE_USER="${USER:-$(id -un)}"

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
    --version)      VERSION="$2";      shift 2 ;;
    --install-dir)  INSTALL_DIR="$2";  shift 2 ;;
    --service-user) SERVICE_USER="$2"; shift 2 ;;
    -h|--help)      usage ;;
    *)              die "unknown flag: $1 (try --help)" ;;
  esac
done

# ── Detect OS / arch ─────────────────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)          ARCH="amd64" ;;
  aarch64|arm64)   ARCH="arm64" ;;
  *) die "unsupported architecture: $ARCH" ;;
esac

if [[ "$OS" == "darwin" ]]; then
  ASSET_NAME="dune-admin_darwin_universal.tar.gz"
else
  ASSET_NAME="dune-admin_${OS}_${ARCH}.tar.gz"
fi

# ── Resolve version ───────────────────────────────────────────────────────────
if [[ "$VERSION" == "latest" ]]; then
  log "resolving latest release from GitHub…"
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
  [[ -n "$VERSION" ]] || die "could not resolve latest version from GitHub API"
fi

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET_NAME}"

log "config:"
log "  version:       $VERSION"
log "  asset:         $ASSET_NAME"
log "  install dir:   $INSTALL_DIR"
log "  service user:  $SERVICE_USER"
log ""

# ── Sanity checks ────────────────────────────────────────────────────────────
[[ "$(id -u)" -eq 0 ]] && die "run this as a normal user with sudo, not as root directly"
sudo -n true 2>/dev/null || die "this user needs passwordless sudo (or authenticate sudo first)"
id "$SERVICE_USER" >/dev/null 2>&1 || die "service user '$SERVICE_USER' does not exist"
command -v curl >/dev/null || die "curl is required"
command -v tar  >/dev/null || die "tar is required"

if systemctl is-active --quiet dune-admin 2>/dev/null; then
  warn "dune-admin.service is currently active. Stop it before re-running:"
  warn "  sudo systemctl stop dune-admin"
  die  "refusing to swap binary under a running service"
fi

# ── Download release ──────────────────────────────────────────────────────────
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

log "downloading ${ASSET_NAME}…"
curl -fsSL --progress-bar "$DOWNLOAD_URL" -o "$tmp/release.tar.gz" \
  || die "download failed — check that $VERSION exists: https://github.com/${REPO}/releases"

mkdir -p "$tmp/extract"
tar -xzf "$tmp/release.tar.gz" -C "$tmp/extract"

# Binary may be at the archive root or one directory deep (goreleaser default)
BINARY="$(find "$tmp/extract" -name "dune-admin" -type f | head -1)"
[[ -n "$BINARY" ]] || die "could not find dune-admin binary in release archive"
EXTRACT_ROOT="$(dirname "$BINARY")"
ok "downloaded $(du -sh "$BINARY" | awk '{print $1}') binary ($VERSION)"
log ""

# ── Install into $INSTALL_DIR ─────────────────────────────────────────────────
log "installing into $INSTALL_DIR…"
sudo mkdir -p "$INSTALL_DIR"
sudo chown "$SERVICE_USER:$SERVICE_USER" "$INSTALL_DIR"

if [[ -f "$INSTALL_DIR/dune-admin" ]]; then
  sudo cp -f "$INSTALL_DIR/dune-admin" "$INSTALL_DIR/dune-admin.prev"
fi
sudo install -m 0755 -o "$SERVICE_USER" -g "$SERVICE_USER" \
  "$BINARY" "$INSTALL_DIR/dune-admin"

for f in item-data.json quality-data.json tags-data.json \
          gameplayTags.json skillModules.json vehicles.json cheatScripts.json; do
  if [[ -f "$EXTRACT_ROOT/$f" ]]; then
    sudo install -m 0644 -o "$SERVICE_USER" -g "$SERVICE_USER" \
      "$EXTRACT_ROOT/$f" "$INSTALL_DIR/$f"
  fi
done
ok "installed: $(ls -la "$INSTALL_DIR/dune-admin" | awk '{print $NF, $5, "bytes"}')"
log ""

# ── systemd unit ──────────────────────────────────────────────────────────────
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
  sudo systemctl restart dune-admin \
    || warn "restart failed; check: sudo journalctl -u dune-admin -e"
fi
ok "systemd unit installed (Restart=always)"
log ""

# ── Seed minimal config.yaml (listen address only) ────────────────────────────
SERVICE_HOME="$(getent passwd "$SERVICE_USER" | cut -d: -f6)"
CONFIG_DIR="$SERVICE_HOME/.dune-admin"
CONFIG_FILE="$CONFIG_DIR/config.yaml"
if [[ ! -f "$CONFIG_FILE" ]]; then
  sudo -u "$SERVICE_USER" mkdir -p "$CONFIG_DIR"
  sudo -u "$SERVICE_USER" tee "$CONFIG_FILE" >/dev/null <<YAML
# Minimal bootstrap config — dune-admin imports this once into its DB on first
# boot, then reads everything from the DB. Run './dune-admin -setup' to fill
# in the remaining fields (DB, AMP, broker, etc.) before starting the service.
listen_addr: :18080
YAML
  ok "wrote $CONFIG_FILE (listen_addr: :18080)"
  log "  Run './dune-admin -setup' before starting the service to complete configuration."
else
  log "existing $CONFIG_FILE found — skipping seed"
fi
log ""

# ── Next steps ────────────────────────────────────────────────────────────────
cat <<EOF

──────────────────────────────────────────────────────────────────────────────
 install complete. dune-admin $VERSION is in $INSTALL_DIR.

 NEXT STEPS (each is intentionally manual so you can review):

 1) RUN THE SETUP WIZARD to generate ~/.dune-admin/config.yaml

      cd $INSTALL_DIR
      ./dune-admin -setup

    Select 'amp' as the control plane. Have these handy:
      - AMP instance name (e.g. DuneAwakening01) — run \`sudo -u amp ampinstmgr -l\`
      - OS user that runs AMP (typically 'amp')
      - PostgreSQL password (set during AMP instance creation)

 2) APPLY SUDOERS GRANTS — the wizard prints an example at the end.
    Save it to /etc/sudoers.d/dune-admin and validate:

      sudo visudo -c

 3) START THE SERVICE — the systemd unit is already installed.
    After the setup wizard has written the config, enable and start it:

      sudo systemctl enable --now dune-admin
      sudo journalctl -u dune-admin -f       # tail logs

    Browse to http://<this-host>:18080 (or whatever listen_addr you chose in -setup).

    NOTE: the unit is written with Restart=always, which is required for
    in-app self-update (Settings → Check for Updates) to restart cleanly.

 ROLLBACK (if something is wrong):

      sudo systemctl stop dune-admin
      sudo cp $INSTALL_DIR/dune-admin.prev $INSTALL_DIR/dune-admin
      sudo systemctl start dune-admin

──────────────────────────────────────────────────────────────────────────────
EOF
