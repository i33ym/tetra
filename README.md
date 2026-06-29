# tetra

A scalable, observable backend service that ingests **payloads** — text and/or
file uploads (documents, images, video) — over a REST API, stores them, and
processes them **asynchronously** via an external (mocked) processor.

It is built as a clean, layered Go service (`api` → `app` → `business` →
`foundation`) on a modern stack:

| Concern        | Choice |
|----------------|--------|
| Routing        | [go-chi/chi](https://github.com/go-chi/chi) (value-returning handlers with a single response choke point) |
| Database       | [pgx/v5](https://github.com/jackc/pgx) (`pgxpool`, native) |
| Config         | [BurntSushi/toml](https://github.com/BurntSushi/toml) + `TETRA_*` env overrides |
| Migrations     | [golang-migrate](https://github.com/golang-migrate/migrate) numbered up/down files |
| Object storage | [MinIO](https://min.io) (S3-compatible) |
| Queue          | Postgres `SELECT … FOR UPDATE SKIP LOCKED` |
| Observability  | OpenTelemetry → OTel Collector → Jaeger (traces); Prometheus + Grafana (metrics) |
| Orchestration  | Docker Compose + Kubernetes (kustomize) with HPA |

## Architecture

```
                 ┌──────────── tetra (API) ─────────────┐
  client ─POST─▶ │ stream file ─▶ MinIO                  │
                 │ tx: INSERT payloads + INSERT jobs     │ ─202─▶ {id, status:pending}
                 └───────────────────────────────────────┘
                                  │ (Postgres queue)
                 ┌──────────── worker ───────────────────┐
                 │ dequeue (FOR UPDATE SKIP LOCKED)       │
                 │ ─HTTP(traced)─▶ mockproc               │
                 │ tx: payloads(done|failed) + result     │
                 └───────────────────────────────────────┘

  traces ─OTLP─▶ otel-collector ─▶ Jaeger
  metrics  /metrics ◀─ scrape ─ Prometheus ─▶ Grafana
```

- **API and worker scale independently** — the API on request load, the worker
  on queue depth/CPU — which is why they are separate deployments sharing the
  same domain packages.
- The queue is a Postgres table; payload + job are inserted in **one
  transaction**, so enqueue is atomic with the metadata write.
- `SKIP LOCKED` lets any number of worker replicas claim disjoint jobs without
  blocking; a `locked_until` lease reclaims jobs from crashed workers
  (at-least-once delivery, exponential backoff on failure).

## API

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v1/payloads` | Ingest a payload. `multipart/form-data` (`file` and/or `text` fields) or `application/json` (`{"text":"..."}`). Returns **202** `{id, status}`. |
| `GET`  | `/v1/payloads/{id}` | Payload status and result. |
| `GET`  | `/v1/payloads` | Paged list. Query: `page`, `rows`, `orderBy` (`id\|status\|kind\|dateCreated`), `status`, `kind`. |
| `GET`  | `/v1/payloads/{id}/content` | Stream the original uploaded file. |
| `GET`  | `/v1/liveness`, `/v1/readiness` | Health (readiness pings Postgres + MinIO). |

`/metrics` (Prometheus) and pprof/expvar are served on the debug port (`:3010`).

## Quick start (Docker Compose)

```bash
make compose/up      # builds images; starts db, minio, collector, jaeger, prometheus, grafana, app, worker, mockproc
make compose/logs    # follow logs
```

Then:

```bash
# text payload
curl -s -X POST localhost:3000/v1/payloads \
  -H 'Content-Type: application/json' -d '{"text":"hello tetra"}'

# file (+ optional text) payload
curl -s -X POST localhost:3000/v1/payloads -F file=@./README.md -F text=note

# poll status (replace ID)
curl -s localhost:3000/v1/payloads/<id>

# download the original file
curl -s localhost:3000/v1/payloads/<id>/content -o out.bin
```

UIs: **Jaeger** http://localhost:16686 · **Prometheus** http://localhost:9090 ·
**Grafana** http://localhost:3002 (anonymous admin; "Tetra Service" dashboard) ·
**MinIO console** http://localhost:9101 (`minioadmin`/`minioadmin`).

```bash
make compose/down    # stop and wipe volumes
```

## Local development (no Docker for the app)

With Postgres, MinIO and an OTLP collector reachable on localhost (or set
`TETRA_OTEL_HOST=""` to disable tracing):

```bash
make dev/tools                 # install migrate, staticcheck, govulncheck
make db/migrations/up          # apply migrations
make minio/bootstrap           # create the bucket
make run/mockproc &            # mock processor on :7000
make run/worker &              # background worker
make run/api                   # API on :3000
```

Set `inproc = true` under `[worker]` in `config.toml` (or `TETRA_WORKER_INPROC=true`)
to run the worker inside the API process for a single-process loop.

Create a migration:

```bash
make db/migrations/new name=add_some_table
```

## Kubernetes (KIND)

```bash
make kind/up      # create cluster + install metrics-server (HPA needs it)
make dev/load     # build images and load them into KIND
make dev/apply    # apply zarf/k8s/dev (kustomize)
make dev/status   # pods, HPAs, services in namespace tetra-system
```

Everything lands in namespace `tetra-system`. The API and worker run under
`HorizontalPodAutoscaler`s (the Deployments omit `replicas` so the HPA owns the
count); Postgres and MinIO are StatefulSets with PVCs. Reach the UIs via
port-forward, e.g.:

```bash
kubectl -n tetra-system port-forward svc/tetra 3000:3000
kubectl -n tetra-system port-forward svc/grafana 3002:3000
```

## Load test

A reproducible harness using [`hey`](https://github.com/rakyll/hey) lives in
[`zarf/loadtest/loadtest.sh`](zarf/loadtest/loadtest.sh) and is wired into the
Makefile. With the stack up (`make compose/up`):

```bash
make dev/tools                                          # installs hey (among others)
docker compose -f zarf/compose/docker-compose.yaml up -d --scale worker=4

make load/text      # JSON ingestion burst        (vars: N, C, HOST)
make load/upload    # multipart file upload -> MinIO (vars: N, C, SIZE, HOST)
make load/watch     # poll payload counts until the queue drains
make load/all       # text + upload, then watch

# example: bigger burst of 1 MiB uploads
N=1000 C=50 SIZE=1048576 make load/upload
```

Representative results on a single Docker host (4 worker replicas × concurrency
8; mock processor latency 0.2–1.5 s with a 10% failure rate):

| Stage | Result |
|-------|--------|
| **Text ingestion** (`POST` JSON) | **7,071 req/s**, 0 failures, p50 **6 ms** / p95 **9 ms** / p99 **20 ms** |
| **Upload ingestion** (multipart, 256 KiB → MinIO) | **~820 req/s**, 500/500 `202`, avg **24 ms**, peak process RSS **~42 MiB** |
| **Async processing** | sustained **~27 jobs/s**; job latency p50 0.83 s / p95 2.3 s |
| **Correctness** | every job reached **exactly one terminal state** — 0 lost / stuck / double-processed across all replicas |
| **Resilience** | the ~10% transient processor failures were retried with exponential backoff and all eventually succeeded (0 permanently failed) |

Takeaways: the async design keeps ingestion fast and flat regardless of
processing cost; processing throughput scales ~linearly with worker replicas
because `SKIP LOCKED` lets them share one queue without contention or
double-processing; the bottleneck is `worker.concurrency × replicas ÷ processor
latency` — tune those to the real processor (in k8s the worker HPA does this
automatically). Uploads are memory-bounded: small objects use a single
exact-sized PUT and only genuinely large objects fall back to multipart
streaming, so a burst of concurrent uploads stays flat on RSS.

Ingestion is **database-connection-pool bound** — it's the primary ingest knob.
In testing, raising `db.max_open_conns` from 10 → 50 lifted single-instance
throughput ~65% (≈4.4K → 7.2K req/s) at the same concurrency while lowering
latency; the default is set to 25. Past that, scale out with more API replicas
and front Postgres with a pooler (e.g. PgBouncer) so total connections stay
under the server's `max_connections`.

## Configuration

`config.toml` holds defaults; any value is overridable by a `TETRA_*` env var
(so the same image works in compose and k8s). Key env vars: `TETRA_DB_HOST`,
`TETRA_DB_PASSWORD`, `TETRA_MINIO_ENDPOINT`, `TETRA_MINIO_ACCESS_KEY`,
`TETRA_MINIO_SECRET_KEY`, `TETRA_OTEL_HOST` (empty disables tracing),
`TETRA_PROCESSOR_URL`, `TETRA_WORKER_INPROC`.

## Layout

```
api/services/{tetra,worker,mockproc}   service entrypoints
api/tooling/admin                      migrations + bucket bootstrap
app/domain/{payloadapp,checkapp}       HTTP handlers (translation layer)
app/sdk/{mux,mid,errs,query,metrics,debug,worker}   shared app machinery
business/domain/{payloadbus,jobbus}    domain logic + pgx stores
business/sdk/{sqldb,delegate,order,page,processor}  persistence + helpers
business/types/status                  value objects
foundation/{web,otel,logger,config,blob}   framework primitives
migrations/                            numbered SQL up/down (embedded)
zarf/{docker,compose,k8s}              build & deploy
```

## Notes

- Metrics are exposed with `prometheus/client_golang` and scraped directly;
  traces go OTLP → collector → Jaeger. The collector is the central fan-out hub
  (add exporters there without touching services).
- Upload requests stream straight to MinIO (no full-file buffering) with a size
  cap enforced mid-stream; metadata lives in Postgres.
- The processing step is a **mock** external HTTP service (`mockproc`) with
  simulated latency and a configurable failure rate to exercise the retry path.
