#!/usr/bin/env bash
#
# Build and restart everything from a checkout on the VM.
#
#   sudo -u enerzia /srv/enerzia/current/deploy/deploy.sh
#
# Idempotent: safe to re-run. Builds first and only restarts once every build
# has succeeded, so a compile error leaves the running site untouched.
set -Eeuo pipefail

# /etc/profile.d only applies to LOGIN shells. `sudo -u enerzia ...` does not get
# one, so Go would not be on PATH and the build fails with "go: not found".
# Set it here so the script works however it is invoked.
export PATH="/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin"

ROOT="${ROOT:-/srv/enerzia/current}"
API_BASE="${API_BASE:-https://api.enerzeiafuturefarm.com/api/v1}"

log() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
fail() { printf '\033[31mFAILED: %s\033[0m\n' "$*" >&2; exit 1; }

[ -d "$ROOT" ] || fail "$ROOT does not exist"

# Fail early with a message that says what to install, rather than part-way
# through a build with a bare "not found".
for bin in go node npm curl; do
  command -v "$bin" >/dev/null 2>&1 || fail "$bin is not on PATH — see runbook section 2"
done
echo "  go   $(go version | awk '{print $3}')"
echo "  node $(node --version)"

cd "$ROOT"

log "Building the API"
cd "$ROOT/enerzia-be"
mkdir -p bin
# Trimmed paths and no symbol table: smaller binary, and build paths stay out
# of any panic that reaches a log.
go build -trimpath -ldflags "-s -w" -o bin/api ./cmd/api
[ -x bin/api ] || fail "the API binary was not produced"

log "Building the storefront"
cd "$ROOT/Enerzia"
npm ci --no-audit --no-fund
# NEXT_PUBLIC_* is inlined at build time. Setting it only in systemd would ship
# a bundle that still calls http://localhost:8080 from a customer's browser.
NEXT_PUBLIC_API_BASE_URL="$API_BASE" npm run build

log "Building the admin console"
cd "$ROOT/enerzia-admin"
npm ci --no-audit --no-fund
NEXT_PUBLIC_API_BASE_URL="$API_BASE" npm run build

log "Restarting services"
sudo systemctl restart enerzia-api
sudo systemctl restart enerzia-shop
sudo systemctl restart enerzia-admin

log "Waiting for health"
for i in $(seq 1 30); do
  if curl -fsS -o /dev/null http://127.0.0.1:8080/health; then
    echo "  API healthy after ${i}s"
    break
  fi
  [ "$i" -eq 30 ] && fail "the API did not become healthy — sudo journalctl -u enerzia-api -n 50"
  sleep 1
done

for port in 3100 3001; do
  for i in $(seq 1 30); do
    if curl -fsS -o /dev/null "http://127.0.0.1:$port/"; then
      echo "  port $port responding"
      break
    fi
    [ "$i" -eq 30 ] && fail "nothing on port $port — check journalctl"
    sleep 1
  done
done

log "Done"
