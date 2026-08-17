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

# Build-time values for the storefront. NEXT_PUBLIC_* is inlined into the
# bundle by `next build`, so these must be present HERE — setting them in
# systemd changes nothing, because the value was already baked in.
#
# They are not secrets: anything NEXT_PUBLIC_ ships to every browser. The file
# exists so they are not committed, not because they are private.
BUILD_ENV="${BUILD_ENV:-/etc/enerzia/build.env}"
if [ -r "$BUILD_ENV" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$BUILD_ENV"
  set +a
fi

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
# Fail here rather than shipping a bundle where these are `undefined`. That
# failure is silent at build time and only shows up as sign-in not working,
# with nothing in any log to explain it.
for v in NEXT_PUBLIC_MSG91_WIDGET_ID NEXT_PUBLIC_MSG91_TOKEN_AUTH; do
  [ -n "${!v:-}" ] || fail "$v is not set — put it in $BUILD_ENV, or sign-in will not work"
done

cd "$ROOT/Enerzia"
npm ci --no-audit --no-fund
NEXT_PUBLIC_API_BASE_URL="$API_BASE" npm run build

log "Building the admin console"
cd "$ROOT/enerzia-admin"
npm ci --no-audit --no-fund
NEXT_PUBLIC_API_BASE_URL="$API_BASE" npm run build

log "Restarting services"
sudo systemctl restart enerzia-api
sudo systemctl restart enerzia-shop
sudo systemctl restart enerzia-admin

log "Waiting for health (a few seconds of silence here is normal)"
for i in $(seq 1 30); do
  # -s not -sS: a service still starting is expected to refuse the first few
  # connections, and printing a curl error each time reads like a failure.
  if curl -fs -o /dev/null http://127.0.0.1:8080/health; then
    echo "  API healthy after ${i}s"
    break
  fi
  [ "$i" -eq 30 ] && fail "the API did not become healthy — sudo journalctl -u enerzia-api -n 50"
  sleep 1
done

for port in 3100 3001; do
  for i in $(seq 1 30); do
    if curl -fs -o /dev/null "http://127.0.0.1:$port/"; then
      echo "  port $port responding"
      break
    fi
    [ "$i" -eq 30 ] && fail "nothing on port $port — check journalctl"
    sleep 1
  done
done

log "Done"
