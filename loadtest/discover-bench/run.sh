#!/usr/bin/env bash
# Times the session discover query (DiscoverSessions) against a realistic number of sessions
# (loadtest/README.md): the load test itself has 40, all near Zagreb, so it cannot show how the
# query's cost grows with data.
#
#   loadtest/discover-bench/run.sh
#   SIZES="40 1000 10000 100000" RUNS=7 loadtest/discover-bench/run.sh
#
# SIZES are the numbers of upcoming public sessions; every size also gets 4x that many other rows
# (history, cancelled, private). Everything happens in a throwaway database (bench_discover) created
# in the compose Postgres and dropped at the end - your data is not touched. Needs the compose stack
# up (task docker:up). The times are this machine's, so read them as relative growth, not as what
# the production host will do.
set -euo pipefail

cd "$(dirname "$0")/../.."

SIZES=${SIZES:-"40 1000 10000 100000"}
RUNS=${RUNS:-7}
CONTAINER=${CONTAINER:-rekreativko_db}
DB=bench_discover
REQUESTER=11111111-1111-1111-1111-111111111111
HERE=loadtest/discover-bench
SCHEMA=activity/internal/infrastructure/persistence/migrations/000001_activity_schema.up.sql

DB_USER=$(docker exec "$CONTAINER" printenv POSTGRES_USER)
psql_db() { local db=$1; shift; docker exec -i -e PGOPTIONS="-c client_min_messages=warning" "$CONTAINER" psql -U "$DB_USER" -d "$db" -X -q -v ON_ERROR_STOP=1 "$@"; }

cleanup() { psql_db postgres -c "DROP DATABASE IF EXISTS $DB" >/dev/null 2>&1 || true; }
trap cleanup EXIT

psql_db postgres -c "DROP DATABASE IF EXISTS $DB" -c "CREATE DATABASE $DB" >/dev/null
psql_db "$DB" -c 'CREATE EXTENSION IF NOT EXISTS "uuid-ossp"' -c 'CREATE EXTENSION IF NOT EXISTS citext' >/dev/null
psql_db "$DB" <"$SCHEMA" >/dev/null

# name|lat|lng|radius km|interest clause
INTERESTS="AND (activity_type = 'football' OR (activity_type = 'running' AND difficulty_level = 'beginner'))"
SCENARIOS=(
  "Zagreb 50km (the load test's)|45.815|15.982|50|"
  "Zagreb 10km|45.815|15.982|10|"
  "Zagreb 50km + 2 interests|45.815|15.982|50|$INTERESTS"
  "Split 50km|43.508|16.440|50|"
  "Osijek 50km|45.555|18.696|50|"
  "Gospic 50km (rural, same latitude band as Zadar)|44.546|15.374|50|"
)

median() { sort -n | awk '{a[NR]=$1} END {print (NR%2 ? a[(NR+1)/2] : (a[NR/2]+a[NR/2+1])/2)}'; }

printf '%-9s %-10s %-48s %6s %9s %10s %9s  %s\n' upcoming rows scenario found "ms (med)" candidates buffers scan
last_plan=""
for n in $SIZES; do
  seed=$(psql_db "$DB" -At -F' ' -v n="$n" -v requester="$REQUESTER" <"$HERE/seed.sql" | tail -1)
  rows_total=${seed%% *}

  for scenario in "${SCENARIOS[@]}"; do
    IFS='|' read -r name lat lng radius interests <<<"$scenario"
    times=()
    plan=""
    for _ in $(seq 1 "$RUNS"); do
      plan=$(psql_db "$DB" -At -v lat="$lat" -v lng="$lng" -v radius="$radius" -v requester="$REQUESTER" \
        -v "interests=$interests" <"$HERE/query.sql")
      times+=("$(awk '/Execution Time:/ {print $3}' <<<"$plan")")
    done

    found=$(grep -m1 -o 'rows=[0-9]*' <<<"$plan" | head -1 | cut -d= -f2)
    scan=$(grep -m1 -E 'Index Scan|Bitmap Heap Scan|Seq Scan|Index Only Scan' <<<"$plan" | sed -E 's/^[ ->]*//; s/ +\(cost.*//')
    scanned=$(grep -m1 -E 'Index Scan|Bitmap Heap Scan|Seq Scan' <<<"$plan" | grep -o 'actual time=[^ ]* rows=[0-9]*' | grep -o 'rows=[0-9]*' | cut -d= -f2)
    removed=$(grep -m1 'Rows Removed by Filter' <<<"$plan" | grep -o '[0-9]*$' || true)
    buffers=$(grep -m1 'Buffers:' <<<"$plan" | awk '{s=0; for(i=1;i<=NF;i++) if ($i ~ /^(hit|read)=/) {split($i,p,"="); s+=p[2]}; print s}')
    candidates=$(( ${scanned:-0} + ${removed:-0} ))
    ms=$(printf '%s\n' "${times[@]}" | median)

    printf '%-9s %-10s %-48s %6s %9s %10s %9s  %s\n' "$n" "$rows_total" "$name" "$found" "$ms" "$candidates" "$buffers" "$scan"
    if [ "$name" = "Zagreb 50km (the load test's)" ]; then last_plan=$plan; fi
  done
done

echo
echo "Plan of the first scenario at the largest size:"
echo
echo "$last_plan"
