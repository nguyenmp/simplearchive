# simplearchive

I love ArchiveBox, but I'm a bit frustrated by how slow it is, the difficulty in debugging, and how complicated the 0.9 beta release is.  I'm a single user who wants to archive content I like or want to find one day.  I'm running ArchiveBox on a cheap VPS and it's too demanding!

I also want the archiving steps to be configurable!

## Technical Overview

Compatible with ArchiveBox's file format, reusing their 0.7/0.8 directory structure and index.json, allowing things to be reimported into ArchiveBox if I give up on this project:
* archive/{timestamp}
* index.json
* We do not write to the ArchiveBox SQLite database

Backend in `go` for small binary footprint that also supports building web servers very well.
* `chi` HTTP server
* slog for structured logging, no metrics or traces for now
* initially simple, w/ 2 goroutines: one for the HTTP server, one for the task queue

Frontend will be HTMX+ templates + tailwind, no SPA.  No need for a SPA.

Task engine:
* goroutines
* SQLite backed task queue
* Retry + backoff + DLQ
* Per-job step logger

Extractors:
* wget
* chromedp / headless_shell -> screenshot, PDF, DOM
* obelisk -> single file HTML
* curl
* yt-dlp -> transcripts, youtube metadata, audio, video

Everything gets built into a docker image that gets deployed.

## Data Model

`meta.db` is a single SQLite database (WAL, single writer) holding the archive index. The on-disk `archive/{timestamp}/` directories and per-snapshot `index.json` files are the ArchiveBox-compatible serialization; `meta.db` is the source of truth for queryable state.

### snapshots

One row per archived URL submission. `id` is the internal surrogate primary key used by foreign keys; `timestamp` is the ArchiveBox "seconds.microseconds" directory name (unique) used for routes and `index.json`.

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `timestamp` INTEGER NOT NULL UNIQUE — ArchiveBox dir name / route key / index.json key
- `url` TEXT NOT NULL
- `title` TEXT
- `created_at`, `updated_at` INTEGER NOT NULL (epoch microseconds)

Snapshot status and `is_archived` are **not stored** — they are derived from the snapshot's `extractor_runs` (see [Deferred](#milestones)). Imported snapshots (no `extractor_runs` rows) are treated as succeeded.

### extractor_runs

One row per extractor run for a snapshot (`wget`, `wget-favicon`, `headers`, `obelisk`, `ytdlp`, `chromedp`). The unit of work **and** the unit of retry. Steps are independent — no primary-fatal, no cancellation (a future bulk `UPDATE … SET status='skipped' WHERE status='pending'` can cancel if ever needed).

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `snapshot_id` INTEGER NOT NULL REFERENCES snapshots(id)
- `extractor` TEXT NOT NULL — `Extractor.Name()`
- `status` TEXT NOT NULL — `pending` | `running` | `succeeded` | `failed` | `skipped`
- `started_at`, `finished_at` INTEGER (NULL until the step runs)
- `error` TEXT

A retry is a **new** `extractor_runs` row (same snapshot + extractor, higher `id`); the latest attempt (max `id`) is the current state.

### step_outputs

One row per output an extractor run produced (wget → `dom` + `favicon`; chromedp → `screenshot` + `pdf` + `chromedp_dom`).

- `id` INTEGER PRIMARY KEY AUTOINCREMENT
- `run_id` INTEGER NOT NULL REFERENCES extractor_runs(id)
- `name` TEXT NOT NULL — `Step.Name` (the ArchiveBox extractor/plugin key)
- `filename` TEXT, `cmd` TEXT (JSON), `status` TEXT NOT NULL
- `start_ts`, `end_ts` INTEGER, `error` TEXT

### Relationships

- snapshots 1—N extractor_runs (by `snapshot_id`)
- extractor_runs 1—N step_outputs (by `run_id`)
- `index.json` is a projection of terminal `extractor_runs` + `step_outputs`, rebuilt per extractor finish (so it is crash-safe and resumable).

## Local Development

We develop inside Docker so the only host dependency is Docker itself (no need to install Go, wget, yt-dlp, etc. on the host machine). The same `Dockerfile.dev` doubles as the basis for the production image later.

Workflow:
1. `docker build -f Dockerfile.dev -t simplearchive-dev .` — builds the dev image with Go toolchain + extractors.
2. `docker run --rm -v "$PWD:/app" -w /app simplearchive-dev go test ./...` — run tests.
3. `docker run --rm -it -v "$PWD:/app" -w /app -p 8080:8080 simplearchive-dev go run .` — run the server with live source mounted. Set `-e LOG_LEVEL=debug` for verbose logs (see [Environment Variables](#environment-variables)).
4. `docker run --rm -it -v "$PWD:/app" -w /app simplearchive-dev` — drop into a shell for `go mod tidy`, `go build`, etc.

The dev container keeps the module cache in a named volume (`go-mod`) so rebuilds are fast. Source is bind-mounted to `/app` so edits on the host are reflected instantly — no rebuild needed for code-only changes.

### chromedp build tag

The chromedp (headless Chromium) extractor is opt-in via the `chromedp` build tag. Without it the extractor compiles to a no-op stub that skips, so the default build needs no Chromium. The dev image ships `chromium`, so build and run with the tag to enable screenshot/PDF/DOM extraction:

```
docker run --rm -v "$PWD:/app" -w /app simplearchive-dev go test -tags chromedp ./...
docker run --rm -it -v "$PWD:/app" -w /app -p 8080:8080 simplearchive-dev go run -tags chromedp .
```

At runtime the extractor still skips (records no steps) if the `chromium` binary is not on `PATH`, so the same binary runs with or without the tag. Network restriction for the browser process is deferred to container-level isolation (see [Production deployment](#production-deployment)).

I often have [the official ArchiveBox repo](https://github.com/ArchiveBox/ArchiveBox) checked out as a sibling as ~/ArchiveBoxOfficial/ for reference.

Commits should be very small and self-contained.  Tasks should be broken up into many small understandable commits.  Not one commit per end-goal.

Testing integration with ArchiveBox:
```
# Make filesystem sandbox
cd $(mktemp -d)
mkdir -p archivebox-data
cp -r ~/simplearchive/archive/ archivebox-data/archive/

# Launch ArchiveBox service
docker run -p 8000:8000 -v ./archivebox-data:/data archivebox/archivebox:stable

# Get shell
docker run -it -v ./archivebox-data:/data archivebox/archivebox:stable /bin/sh
$ archivebox init
```

## Environment Variables

| Variable     | Default          | Description                                                                                          |
| ------------ | ---------------- | ---------------------------------------------------------------------------------------------------- |
| `LOG_LEVEL`  | `info`           | Structured log level for the JSON slog handler. One of `debug`, `info`, `warn`, `warning`, `error`. |
| `SERVE_ADDR`  | `127.0.0.1:8080` | Listen address for `simplearchive serve`. |

Pass through to the dev container with `-e`, e.g. `docker run --rm -e LOG_LEVEL=debug ...`.

## Production deployment

`SERVE_ADDR` defaults to `127.0.0.1:8080` (localhost only). For production behind a reverse proxy, bind all interfaces so the proxy can reach it:

```
docker run --rm -it -v "$PWD:/app" -w /app -e SERVE_ADDR=0.0.0.0:8080 simplearchive-dev go run .
```

### Network isolation

Run simplearchive on a dedicated Docker network that only the reverse proxy also joins, and don't publish its port — the proxy reaches it by container name. The proxy can route to simplearchive, but simplearchive can't resolve or route to any other container.

```
networks:
  simplearchive_net:
    driver: bridge

services:
  simplearchive:
    networks: [simplearchive_net]
  caddy:
    networks: [default, simplearchive_net]
```

The server has no built-in auth; rely on the reverse proxy (basic auth or a trusted network) until M4.

## Tailwind CSS

The server embeds a compiled `tailwind.css` (committed under `internal/server/assets/static/`). When you add or change utility classes in a template, regenerate it inside the dev container:

```
docker run --rm -v "$PWD:/app" -w /app simplearchive-dev make css
```

## Milestones

M1 — Sync ingest (CLI)
- [x] Dockerfile stub (builder image with Go + tooling for local dev).
- [x] Go module (`go mod init`) + empty `main.go` that builds inside the Docker dev container.
- [x] slog structured logging wired up (JSON handler, level configurable via env).
- [x] meta.db SQLite: snapshots table only.
- [x] simplearchive add <url>: create row → mkdir archive/{timestamp}/ → wget inline (output.html, favicon.ico, headers.json) → write AB-compatible per-snapshot index.json → update row → print summary.
- [x] Acceptance: re-scan via archivebox init succeeds.

M2 — Read + serve + web add
- [x] Import 706 existing snapshots into meta.db (scan archive/*/index.json).
- [x] simplearchive serve: HTTP server (chi). SQLite WAL + short transactions.
- [x] HTMX list/detail views, tailwind. Add-URL form. Static file server (path-scoped + CSP sandbox + nosniff).
- [x] Local-run only.

M3 — Richer extractors
- [x] Pluggable pipeline: `extractors.Extractor` interface + `Step`/`Result` types; wrap wget/favicon/headers as adapters; refactor `ingest.Add` to run a list of extractors (no behavior change).
- [x] Per-extractor status persistence: migration v2 `extractor_runs` table + queries; `ingest.Add` writes one row per step; introduce `failed` snapshot status when primary DOM fails.
- [x] Per-extractor status in UI: detail page renders a per-extractor status table (read from `extractor_runs`).
- [x] obelisk extractor: single-file HTML (`singlefile.html`), in-process; wire into default list.
- [x] yt-dlp extractor: metadata + transcript only (`--write-info-json --write-subs --skip-download`), host-gated to video sites; add standalone binary to Dockerfile.dev.
- [x] chromedp extractor: screenshot + PDF + DOM, build-tagged (`-tags chromedp`) with a no-op stub when disabled and runtime binary-presence check; add `chromium` to Dockerfile.dev.
- [x] Revisit worker-isolation topology here. Network restriction for chromedp deferred to container-level isolation (see [Production deployment](#production-deployment)).

M3.5 — Worker split (deferred until inline archiving is too slow)
- [x] snapshots surrogate id PK (timestamp demoted to unique).
- [x] Reshape extractor_runs to per-extractor + step_outputs (snapshot_id FK).
- [x] Drop snapshots.status + is_archived (derive from per-step state).
- [x] Enqueue + RunSnapshot core: add enqueues pending extractor_runs; RunSnapshot claims a snapshot and runs its steps independently (no primary-fatal), rebuilding index.json per step.
- [ ] serve runs a worker goroutine draining pending snapshots; web Add-URL enqueues async.
- [ ] simplearchive add --wait blocks + streams step logs (preserves sync UX).

M4 — Production
- [ ] Basic auth behind reverse proxy.
- [ ] Docker image, deploy to VPS pointing at archivebox-data/archive/.
- [ ] Per-job step logger view.

M5 — Reliability + config
- [ ] Retry/backoff/DLQ.
- [ ] YAML extractor pipeline config per-URL/tag.

M6 - Search
- [ ] Full-text search across URL, title, content
- [ ] Semantic search across the same

Deferred
- [ ] Derive snapshot status/is_archived from per-step state (latest attempt per extractor).
- [ ] Re-run extractors on existing snapshots.
- [ ] URL validation + response-size cap + timeouts.

## Contributing

This project uses [jj](https://www.jj-vcs.dev/latest/) to work with git.
