#!/usr/bin/env bash
# How fast the database's disk can flush a write: Postgres' own pg_test_fsync, run on the database volume.
#
# Every COMMIT waits for the write-ahead log to reach the disk, so the time of one 8 kB write + flush is the
# floor of a commit's latency, and its inverse is the most commits per second one connection can do. It is
# where VPS tiers and storage types differ most, and neither CPU nor memory numbers show it
# (loadtest/README.md). Run it on each candidate host before deploying.
#
#   loadtest/disk-check.sh                       # the compose database (container rekreativko_db)
#   TARGET=k8s loadtest/disk-check.sh            # the CloudNativePG primary (namespace rekreativko)
#   SECONDS_PER_TEST=10 loadtest/disk-check.sh   # longer, steadier numbers (default 5)
#
# It writes one 16 MB scratch file beside the data directory, on the same volume, and removes it afterwards.
set -euo pipefail

TARGET=${TARGET:-compose}
SECONDS_PER_TEST=${SECONDS_PER_TEST:-5}
SCRATCH=/var/lib/postgresql/data/pg_test_fsync.scratch

case "$TARGET" in
  compose)
    CONTAINER=${CONTAINER:-rekreativko_db}
    run() { docker exec "$CONTAINER" "$@"; }
    where="container $CONTAINER"
    ;;
  k8s)
    NAMESPACE=${NAMESPACE:-rekreativko}
    POD=$(kubectl get pod -n "$NAMESPACE" -l cnpg.io/cluster=rekreativko-db,cnpg.io/instanceRole=primary \
      -o jsonpath='{.items[0].metadata.name}')
    [ -n "$POD" ] || { echo "no CloudNativePG primary pod found in namespace $NAMESPACE" >&2; exit 1; }
    run() { kubectl exec -n "$NAMESPACE" -c postgres "$POD" -- "$@"; }
    where="pod $POD"
    ;;
  *)
    echo "TARGET must be compose or k8s" >&2
    exit 1
    ;;
esac

cleanup() { run rm -f "$SCRATCH" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "pg_test_fsync on the data volume of $where, ${SECONDS_PER_TEST}s per test"
echo

run pg_test_fsync -s "$SECONDS_PER_TEST" -f "$SCRATCH"

cat <<'EOF'

Read the "one 8kB write" block: the line for fdatasync (Postgres' default on Linux) is what a commit costs.
usecs/op is the latency of one commit's flush; ops/sec is how many one connection can do in a second
(concurrent commits share flushes, so the server as a whole does more). Compare candidate hosts and storage
types by it: a number that is a few times lower than the others is the host to avoid for the database,
whatever its CPU and RAM.
EOF
