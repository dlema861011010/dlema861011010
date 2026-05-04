#!/usr/bin/env bash
# scripts/dev-setup.sh — one-shot setup for all environments:
#   VS Code Dev Container, GCP Cloud Shell, Termux (Android), and bare Linux.
#
# Usage:
#   bash scripts/dev-setup.sh
#
# On Termux:
#   pkg install golang nodejs python git && bash scripts/dev-setup.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OS="$(uname -s)"
ARCH="$(uname -m)"
IN_TERMUX="${TERMUX_VERSION:-}"

log() { printf '\033[0;36m[fuseflash]\033[0m %s\n' "$*"; }
warn() { printf '\033[0;33m[fuseflash] WARN:\033[0m %s\n' "$*"; }

# ── Go ──────────────────────────────────────────────────────────────────────
setup_go() {
  log "Setting up Go module (fuseflash)…"
  cd "$ROOT/fuseflash"
  go mod tidy
  go vet ./...
  go test ./... -timeout 60s
  log "Go: ✓"
}

# ── Python bridge ────────────────────────────────────────────────────────────
setup_python() {
  log "Setting up Python bridge…"
  cd "$ROOT/bridge"
  if command -v python3 &>/dev/null; then
    python3 -m venv .venv 2>/dev/null || true
    # shellcheck disable=SC1091
    source .venv/bin/activate 2>/dev/null || true
    pip install --quiet --upgrade pip
    pip install --quiet -r requirements.txt
    log "Python: ✓"
  else
    warn "python3 not found — skipping bridge setup."
    if [[ -n "$IN_TERMUX" ]]; then
      warn "On Termux: pkg install python"
    fi
  fi
}

# ── Node.js / Ember ──────────────────────────────────────────────────────────
setup_node() {
  log "Setting up Ember smart-wallet…"
  cd "$ROOT/smart-wallet"
  if command -v node &>/dev/null; then
    NODE_VER=$(node --version | cut -c2- | cut -d. -f1)
    if [[ "$NODE_VER" -lt 18 ]]; then
      warn "Node.js >= 18 required (found $(node --version)). Please upgrade."
      return
    fi
    npm install --prefer-offline --silent
    log "Node: ✓"
  else
    warn "node not found — skipping smart-wallet setup."
    if [[ -n "$IN_TERMUX" ]]; then
      warn "On Termux: pkg install nodejs"
    fi
  fi
}

# ── Docker check ─────────────────────────────────────────────────────────────
check_docker() {
  if command -v docker &>/dev/null; then
    log "Docker: $(docker --version)"
  else
    warn "Docker not found — container builds will be unavailable."
    if [[ -n "$IN_TERMUX" ]]; then
      warn "Docker is not supported in standard Termux. Use --no-docker flag or run on a rooted device."
    fi
  fi
}

# ── Main ─────────────────────────────────────────────────────────────────────
log "FuseFlash dev setup — OS: $OS / Arch: $ARCH"
[[ -n "$IN_TERMUX" ]] && log "Detected Termux environment"

setup_go
setup_python
setup_node
check_docker

log "✓ All components set up. Happy hacking!"
log ""
log "  Go P2P node :  cd fuseflash && go run ./cmd/node"
log "  Python bridge: cd bridge && source .venv/bin/activate && uvicorn fuseflash_bridge:app --reload"
log "  Ember wallet : cd smart-wallet && npm start"
log "  Docker stack : docker compose -f docker/docker-compose.yml up --build"
