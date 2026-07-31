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

## Local Development

We develop inside Docker so the only host dependency is Docker itself (no need to install Go, wget, yt-dlp, etc. on the host machine). The same `Dockerfile.dev` doubles as the basis for the production image later.

Workflow:
1. `docker build -f Dockerfile.dev -t simplearchive-dev .` — builds the dev image with Go toolchain + extractors.
2. `docker run --rm -v "$PWD:/app" -w /app simplearchive-dev go test ./...` — run tests.
3. `docker run --rm -it -v "$PWD:/app" -w /app -p 8080:8080 simplearchive-dev go run .` — run the server with live source mounted. Set `-e LOG_LEVEL=debug` for verbose logs (see [Environment Variables](#environment-variables)).
4. `docker run --rm -it -v "$PWD:/app" -w /app simplearchive-dev` — drop into a shell for `go mod tidy`, `go build`, etc.

The dev container keeps the module cache in a named volume (`go-mod`) so rebuilds are fast. Source is bind-mounted to `/app` so edits on the host are reflected instantly — no rebuild needed for code-only changes.

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

## Serving online

`SERVE_ADDR` defaults to `127.0.0.1:8080`, which only listens on localhost — fine for local dev but not reachable from outside the host. To serve the dev container on the network (e.g. on a VPS, or to test from another machine on your LAN), bind all interfaces and publish the port:

```
docker run --rm -it -v "$PWD:/app" -w /app -p 8080:8080 -e SERVE_ADDR=0.0.0.0:8080 simplearchive-dev go run .
```

Then visit `http://<host-ip>:8080`. This exposes the server with no auth — only use on a trusted network or behind a reverse proxy (see M4).

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
- [ ] chromedp (child process, network-restricted), obelisk, yt-dlp.
- [ ] Per-extractor status in UI. Re-run on existing snapshots.
- [ ] Revisit worker-isolation topology here.

M3.5 — Worker split (deferred until inline archiving is too slow)
- [ ] Add queue_jobs + job_steps tables.
- [ ] add enqueues, returns timestamp (CLI prints id/status).
- [ ] simplearchive worker drains queue (single goroutine, recover() per job).
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
- [ ] URL validation + response-size cap + timeouts.

## Contributing

This project uses [jj](https://www.jj-vcs.dev/latest/) to work with git.
