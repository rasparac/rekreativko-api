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

Results go to `loadtest/results/<timestamp>/` (gitignored), with `report.txt` holding the tables.

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
  logins/s ≈ 240 of its 277 m. Size identity by logins per second (≈ 60 m per login/s), not by total
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
