#!/usr/bin/env bash
#
# Reproducible load tests for tetra using `hey` (https://github.com/rakyll/hey).
# Install hey with:  go install github.com/rakyll/hey@latest   (or: make dev/tools)
#
# Usage:
#   ./zarf/loadtest/loadtest.sh text     # JSON ingestion burst
#   ./zarf/loadtest/loadtest.sh upload   # multipart file-upload burst (-> MinIO)
#   ./zarf/loadtest/loadtest.sh watch    # poll queue/payload counts until drained
#   ./zarf/loadtest/loadtest.sh all      # text + upload, then watch
#
# Tunables (env vars):
#   HOST=localhost:3000   N=2000   C=50   SIZE=262144   (SIZE = upload bytes/file)
set -euo pipefail

HOST="${HOST:-localhost:3000}"
URL="http://${HOST}/v1/payloads"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

# Resolve hey from PATH or the Go bin directory.
HEY="$(command -v hey || true)"
if [ -z "$HEY" ] && command -v go >/dev/null 2>&1; then
  HEY="$(go env GOPATH)/bin/hey"
fi
need_hey() {
  if [ ! -x "$HEY" ]; then
    echo "hey not found. Install it with: go install github.com/rakyll/hey@latest" >&2
    exit 1
  fi
}

count() {
  curl -s "http://${HOST}/v1/payloads?status=$1&rows=1" \
    | python3 -c "import sys,json;print(json.load(sys.stdin)['total'])"
}

load_text() {
  need_hey
  local n="${N:-2000}" c="${C:-50}"
  echo "==> TEXT ingestion: $n requests, concurrency $c -> $URL"
  "$HEY" -n "$n" -c "$c" -m POST -T application/json -d '{"text":"load-test payload"}' "$URL" \
    | grep -E 'Requests/sec|Total:|Slowest|Fastest|Average|status code|\[2|\[4|\[5|0\.(50|90|95|99)'
}

load_upload() {
  need_hey
  local n="${N:-500}" c="${C:-20}" size="${SIZE:-262144}"
  local boundary="----tetraboundary$$"
  local sample="$WORKDIR/sample.dat"
  local body="$WORKDIR/multipart.bin"

  # base64 payload => no "--" bytes, so it can never collide with the boundary.
  # base64 expands ~4/3, so take 3/4 of SIZE raw bytes to land near SIZE.
  local raw=$(( size * 3 / 4 ))
  head -c "$raw" /dev/urandom | base64 | tr -d '\n' > "$sample"

  {
    printf -- '--%s\r\n' "$boundary"
    printf -- 'Content-Disposition: form-data; name="text"\r\n\r\n'
    printf -- 'load-test\r\n'
    printf -- '--%s\r\n' "$boundary"
    printf -- 'Content-Disposition: form-data; name="file"; filename="sample.dat"\r\n'
    printf -- 'Content-Type: application/octet-stream\r\n\r\n'
    cat "$sample"
    printf -- '\r\n--%s--\r\n' "$boundary"
  } > "$body"

  echo "==> UPLOAD ingestion: $n requests, concurrency $c, ${size} bytes/file -> MinIO"
  "$HEY" -n "$n" -c "$c" -m POST -T "multipart/form-data; boundary=${boundary}" -D "$body" "$URL" \
    | grep -E 'Requests/sec|Total:|Slowest|Fastest|Average|status code|\[2|\[4|\[5|0\.(50|90|95|99)'
}

watch_drain() {
  echo "==> draining (pending+processing -> 0)"
  local prev=0 start
  start=$(date +%s)
  for _ in $(seq 1 60); do
    local t p pr d f
    t=$(( $(date +%s) - start ))
    p=$(count pending); pr=$(count processing); d=$(count done); f=$(count failed)
    printf 't=%3ss  pending=%-6s processing=%-5s done=%-6s failed=%-4s (~%s done/s)\n' \
      "$t" "$p" "$pr" "$d" "$f" "$(( (d - prev) / 3 ))"
    prev=$d
    if [ "$p" -eq 0 ] && [ "$pr" -eq 0 ]; then echo ">>> drained"; return; fi
    sleep 3
  done
  echo ">>> stopped sampling (still draining; long tail is retry backoff)"
}

case "${1:-}" in
  text)   load_text ;;
  upload) load_upload ;;
  watch)  watch_drain ;;
  all)    load_text; echo; load_upload; echo; watch_drain ;;
  *) echo "usage: $0 {text|upload|watch|all}" >&2; exit 2 ;;
esac
