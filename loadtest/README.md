# Load test and resource sizing

How much CPU and memory each service needs, measured on the real docker images under a generated load, and
the Kubernetes requests/limits that follow (`rekreativko-api-a66`). Re-run it after changes that could move
the numbers (a new hot endpoint, a dependency upgrade, a Go upgrade) and update the tables below.

## Running it

```bash
task docker:build                 # images of the current code
task loadtest:up                  # services with loadtest/docker-compose.loadtest.yml
task loadtest:seed                # once: 40 users, a session each, 5 RSVPs each
RATES="0 25 50 100 200 400 800" DURATION=90s FIT_MAX_RPS=400 task loadtest:run
task docker:up                    # back to the dev settings afterwards
```

- **Load**: k6 (`k6/load.js`, in docker on the compose network) sends a constant rate of requests to the
  gateway, each picked from a weighted mix close to what the app sends: session discovery and details,
  attendees, team formation, the notification feed, profile and settings, `/me`, a cached reverse geocode,
  6% RSVP writes (which also push events through the outbox, NATS and notifications) and 1% logins.
- **Production-like settings** (`docker-compose.loadtest.yml`): `LOG_LEVEL=info` and 10% trace sampling,
  since debug logs and full tracing cost more CPU than the requests themselves. The gateway's rate limits
  are lifted, as every virtual user comes from one IP.
- **Measuring** (`scripts/resource-sampler`): every second it reads each container's cgroup, the same
  numbers the kubelet uses. CPU is the change in `cpu.stat usage_usec`. Memory is the working set,
  `memory.current - inactive_file`, which is what a memory limit is enforced against. The first 10 s of
  each run are left out, because that is when k6 logs in.
- **Calculating**: for each service, CPU is fitted as `idle + slope × rps`. That line gives the CPU at
  whatever rate a pod is expected to carry. Memory barely moves with rate, so it is sized from the observed
  maximum.

Results go to `loadtest/results/<timestamp>/` (gitignored), with `report.txt` holding the tables: the
per-container CPU and memory, the CPU fit, the latency k6 saw (overall, and p95 per endpoint by rate), and
the whole host (below).

## Is the host big enough: whole-host numbers, logins, the disk

The per-container numbers say what each service needs. Whether the machine they share is big enough is
a different question, and these answer it (they are what to compare between candidate VPSes):

- **Whole host**, sampled every second next to the containers (`summary-node.csv`, printed in the report):
  CPU busy, **steal** (time the hypervisor gave to someone else while this machine had work: how an oversold
  shared-vCPU host shows up) and iowait from `/proc/stat`; **pressure stall information** (`/proc/pressure/*`:
  the share of wall time work waited for CPU, memory or disk, which utilisation alone cannot show); the
  lowest `MemAvailable` of the run and swap in use. A host without PSI says `na` for the pressure columns.
  Steal staying near 0 and memory pressure at 0 while CPU pressure stays low is a host with room.
- **A wave of logins**: `MODE=spike RATES="25 50 100 200" DURATION=10s loadtest/run.sh` (`task loadtest:spike`)
  sends only logins at that many per second. A bcrypt check (cost 10, hardcoded in identity's `main.go`) takes
  tens of milliseconds of one core (measured below), so a host tops out at a handful of logins a second per
  core; past that logins queue, which shows as latency and dropped iterations. This is the spike after a
  release that logs everyone out. The CPU fit in that report is meaningless (it is for the request mix); read
  latency, dropped and identity's CPU.
- **Disk**: `loadtest/disk-check.sh` (`task loadtest:disk`) runs Postgres' `pg_test_fsync` on the database
  volume. Every commit waits for one flush of the write-ahead log, so this is the floor of commit latency and
  where VPS tiers and storage types differ most. `TARGET=k8s` runs it in the CloudNativePG primary.
- **Per-endpoint latency**: the report lists p95 per endpoint by rate, since the overall p99 is just the 1%
  of requests that are logins.

### Measured on the development workstation (2026-11-26)

An 8-core desktop that is itself busy (load average about 7, CPU pressure 44 to 59% before any test starts),
so read these as a baseline to compare a VPS against, not as capacity:

- **Login spike** (only logins, everything on one host):

  | logins per second | achieved | p50 | p95 | dropped | identity CPU | host CPU busy |
  |---:|---:|---:|---:|---:|---:|---:|
  | 10 | 10 | 82 ms | 101 ms | 0 | 0.79 cores | 38% |
  | 30 | 29 | 106 ms | 151 ms | 0 | 2.95 cores | 73% |
  | 60 | 45 | 1,817 ms | 2,301 ms | 54 | 4.72 cores | 99.7% |

  A login costs about 80 ms of identity CPU at a low rate and about 100 ms as the host saturates, more than
  the 60 ms the first (quieter) run suggested. This host stopped keeping up at about 45 logins a second, and
  past it logins queue: at 60 a second the median wait was 1.8 s. As a rough extrapolation, not a
  measurement, a free 4 vCPU host would manage about 30 a second, so a thousand people signing in at once
  (a release that logs everyone out) takes about half a minute to drain, and the first of them see no delay
  at all only if the rate stays below that.
- **Disk**: `pg_test_fsync` on the compose volume, 8 kB write + flush: `fdatasync` 3,800 per second at 263 µs
  (`open_datasync` 4,300, `fsync` 2,300). A VPS whose `fdatasync` is several times slower than this is the one
  to be careful with for the database.
- **Reading the host table**: compare each run to the idle run (rate 0) of the same batch. Here the CPU
  pressure was 44 to 59% in every run, idle included, because the workstation is busy by itself; on a quiet
  VPS it should be near 0 until the load starts. Steal was 0 (this is not a shared virtual CPU), memory
  pressure 0, and at least 16 GB stayed available.

### Running it against a VPS

Not tried yet (there is no VPS): the steps this tooling is meant for.

1. Install the stack on the candidate host (single-node k3s, everything from `rekreativko-infra`), create the
   load-test users there, and lift the gateway's rate limits for the run (as `docker-compose.loadtest.yml`
   does) - then put them back.
2. Run k6 from **another machine** against the host (`BASE_URL=https://...`), so the load generator does not
   compete for its CPU. The compose-based `run.sh` is for one host; for a remote one run the k6 container by
   hand with `-e BASE_URL`, `-e RATE`, `-e DURATION`, `--summary-export`.
3. On the host, sample the node during each run: `go run ./scripts/resource-sampler -containers "" -label
   <rate> -out results/summary.csv`, interrupt it when k6 ends, then `-report`. Per-pod numbers come from
   `kubectl top pods` (the sampler reads docker's cgroups, which k3s does not use).
4. `TARGET=k8s loadtest/disk-check.sh` on the host, and a spike run.

## Results (2026-11-07, Go 1.27.3, 8-core host, everything on one machine)

k6 got what it asked for at every rate, with 0% failures. Latency in ms; the p99 is the bcrypt logins:

| target rps | achieved | p50 | p95 | p99 |
|---:|---:|---:|---:|---:|
| 25 | 26 | 3 | 12 | 58 |
| 100 | 103 | 1 | 2 | 57 |
| 400 | 414 | 1 | 2 | 56 |
| 800 | 826 | 2 | 5 | 30 |

Average CPU in millicores, and the highest working set across all runs:

| service | idle | 100 rps | 400 rps | 800 rps | m per 100 rps (fit ≤ 400) | max working set |
|---|---:|---:|---:|---:|---:|---:|
| gateway | 2 | 83 | 317 | 856 | 71 | 42 MiB |
| identity | 1 | 59 | 277 | 693 | 69 | 18 MiB |
| activity | 2 | 57 | 218 | 622 | 50 | 19 MiB |
| account-profile | 1 | 6 | 23 | 62 | 5 | 14 MiB |
| notifications | 2 | 7 | 26 | 68 | 6 | 15 MiB |
| outbox-publisher | 1 | 2 | 10 | 26 | 2 | 14 MiB |
| location | 0 | 1 | 4 | 9 | 1 | 9 MiB |
| pgbouncer | 2 | 11 | 42 | 130 | 9 | 4 MiB |
| postgres | 4 | 26 | 98 | 292 | 22 | 103 MiB |

What the numbers say:

- **CPU is linear up to 400 rps**. At 800 rps it costs about 30% more per request. That is this one host,
  shared with k6 and Postgres, running out of room, not the services themselves. Running 800 rps again
  with `GOGC=200` left the gateway's CPU the same and grew its memory by 40%, so GC isn't the cause and
  GC tuning isn't worth it.
- **The gateway costs the most per request**, about 0.7 ms of CPU, since every request passes through it.
  That covers JWT verification, proxying, access logging and metrics.
- **identity's CPU is almost entirely logins.** A bcrypt check (cost 10) takes about 60 ms of one core
  (the p99 above), and the 1% of requests that are logins are most of identity's CPU: at 400 rps, 4
  logins/s ≈ 240 of its 277 m. Size identity by logins per second (≈ 60 m per login/s on this first run; a
  later login-spike run on a busier host measured 80 to 105 m, see below), not by total
  traffic, and give it room to burst: a wave of logins, e.g. after
  a release logs everyone out, is a CPU spike.
- **Memory is small and flat.** Every Go service stays under 45 MiB of working set even at 800 rps. Latency
  is low, so few requests are in flight at once. The gateway grows the most (14 → 42 MiB) because it holds
  each proxied request and response.

## Recommended Kubernetes resources

These are per pod, sized for about **200 rps of this mix per pod** (far above early traffic), with
replicas added through the HPA on CPU beyond that. The CPU request is `idle + slope × 200` rounded up; the
memory request is about 2× the observed maximum.

| service | cpu request | cpu limit | memory request | memory limit | `GOMEMLIMIT` |
|---|---:|---:|---:|---:|---:|
| gateway | 200m | none | 96Mi | 192Mi | 160MiB |
| identity | 200m | none | 64Mi | 128Mi | 110MiB |
| activity | 150m | none | 96Mi | 192Mi | 160MiB |
| account-profile | 50m | none | 48Mi | 96Mi | 80MiB |
| notifications | 50m | none | 48Mi | 96Mi | 80MiB |
| outbox-publisher | 25m | none | 48Mi | 96Mi | 80MiB |
| location | 25m | none | 32Mi | 64Mi | 54MiB |

Why it is shaped this way:

- **No CPU limits.** The request is what the scheduler reserves and what the HPA scales on. A limit would
  only throttle bursts that the node has room for: identity's logins, startup, and catching up on
  the outbox. If the cluster's policy requires limits, set at least `1000m`. Go ≥ 1.25 sets `GOMAXPROCS`
  from the container's CPU limit on its own, so neither `automaxprocs` nor a `GOMAXPROCS` env var is
  needed.
- **Memory limit = 2× request, and `GOMEMLIMIT` at about 85% of the limit.** As the heap nears the limit
  the GC works harder rather than the kernel OOM-killing the pod. Set it next to the limit in the same
  manifest, and change both together. A `resourceFieldRef` to `limits.memory` would keep them in step,
  but it can only pass the whole limit, which leaves no room for the memory that isn't Go heap.
- **activity and gateway get more memory headroom** for what this test does not cover: open
  team-formation SSE streams (`/sessions/{id}/events`) each hold a goroutine and buffers, in activity and
  in the gateway that streams them through. Measure that before relying on these numbers for a session
  with hundreds of viewers.
- **Postgres** (about 22 m per 100 rps and 100 MiB on this tiny dataset) and pgbouncer are sized
  elsewhere; on a managed database it is the instance class.
- **activity-cron** is a one-shot job and was not measured. Give it 50m / 64Mi with a 128Mi limit and
  check its peak in `kubectl top` after the first runs.

## What this test does not tell you

- **Scale of data**: 40 users and 40 sessions. Discovery, the feed and attendee lists get more expensive
  as tables grow, so re-run with a realistic dataset before trusting the CPU per request for those.
- **One host**: Postgres, NATS, k6 and every service share 8 cores, with no network between them. Repeat
  `task loadtest:run` against a staging cluster, and compare with `kubectl top pod` and
  `container_cpu_usage_seconds_total` in production.
- **Long-lived connections**: the SSE streams and their memory per viewer, mentioned above.
