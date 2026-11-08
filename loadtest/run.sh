#!/usr/bin/env bash
# Runs the load test at each request rate in RATES and samples every service's
# CPU and memory meanwhile (loadtest/README.md). Expects the stack up with
# loadtest/docker-compose.loadtest.yml (task loadtest:up) and task loadtest:seed
# done.
#
#   RATES="0 25 50 100 200 400" DURATION=90s FIT_MAX_RPS=400 loadtest/run.sh
#
# Rate 0 samples the idle stack. FIT_MAX_RPS leaves the runs above it, where
# this host itself saturates, out of the CPU fit. Results land in loadtest/results/<timestamp>/.
set -euo pipefail

cd "$(dirname "$0")/.."

RATES=${RATES:-"0 25 50 100 200 400"}
DURATION=${DURATION:-90s}
K6_IMAGE=${K6_IMAGE:-grafana/k6:1.0.0}
NETWORK=${NETWORK:-rekreativko-api_rekreativko}
OUT=loadtest/results/$(date -u +%Y%m%dT%H%M%SZ)

mkdir -p "$OUT"
go build -o "$OUT/resource-sampler" ./scripts/resource-sampler

for rate in $RATES; do
  echo "=== ${rate} rps for ${DURATION}"
  "$OUT/resource-sampler" -label "$rate" -out "$OUT/summary.csv" &
  sampler=$!

  if [ "$rate" = 0 ]; then
    # same length as a loaded run, so the idle numbers are comparable
    sleep "$((${DURATION%s} + 10))"
  else
    # a run that misses its thresholds is still recorded - the report shows
    # at which rate the stack stopped keeping up
    docker run --rm --network "$NETWORK" --user "$(id -u):$(id -g)" \
      -v "$PWD/loadtest/k6:/scripts:ro" -v "$PWD/$OUT:/out" \
      -e RATE="$rate" -e DURATION="$DURATION" \
      "$K6_IMAGE" run -q --summary-export "/out/k6-${rate}.json" /scripts/load.js \
      > "$OUT/k6-${rate}.log" 2>&1 || echo "k6 thresholds failed at ${rate} rps (see $OUT/k6-${rate}.log)"
    jq -r --arg rate "$rate" '.metrics as $m | [$rate, ($m.http_reqs.rate | floor),
        ($m.http_req_failed.value * 100 | . * 100 | round / 100), ($m.http_req_duration["p(50)"] | round),
        ($m.http_req_duration["p(95)"] | round), ($m.http_req_duration["p(99)"] | round),
        ($m.dropped_iterations.count // 0)] | @tsv' "$OUT/k6-${rate}.json" >> "$OUT/latency.tsv"
  fi

  kill -INT "$sampler"
  wait "$sampler"
  sleep 5 # let the outbox and consumers drain before the next rate
done

{
  "$OUT/resource-sampler" -report "$OUT/summary.csv" -fit-max-rps "${FIT_MAX_RPS:-0}"
  echo
  echo "Requests as k6 saw them (latency in ms):"
  echo
  printf 'target\tachieved\tfailed%%\tp50\tp95\tp99\tdropped\n'
  cat "$OUT/latency.tsv"
} | tee "$OUT/report.txt"
rm "$OUT/resource-sampler"
