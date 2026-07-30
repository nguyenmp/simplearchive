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

## Milestones

M1 — Sync ingest (CLI)
- [ ] Go module, slog, Dockerfile stub.
- [ ] meta.db SQLite: snapshots table only.
- [ ] simplearchive add <url>: create row → mkdir archive/{timestamp}/ → wget inline (output.html, favicon.ico, headers.json) → write AB-compatible per-snapshot index.json → update row → print summary.
- [ ] URL validation + response-size cap + timeouts (security baseline from day one).
- [ ] Acceptance: re-scan via archivebox init succeeds.

M1.5 — Worker split
- [ ] Add queue_jobs + job_steps tables.
- [ ] add enqueues, returns timestamp (CLI prints id/status).
- [ ] simplearchive worker drains queue (single goroutine, recover() per job).
- [ ] simplearchive add --wait blocks + streams step logs (preserves sync UX).

M2 — Read + serve + web add
- [ ] Import 706 existing snapshots into meta.db (scan archive/*/index.json).
- [ ] simplearchive serve: HTTP server (chi) + embedded worker goroutine. SQLite WAL + short transactions.
- [ ] HTMX list/detail views, tailwind. Add-URL form. Static file server (path-scoped + CSP sandbox + nosniff).
- [ ] Local-run only.

M3 — Richer extractors
- [ ] chromedp (child process, network-restricted), obelisk, yt-dlp.
- [ ] Per-extractor status in UI. Re-run on existing snapshots.
- [ ] Revisit worker-isolation topology here.

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
