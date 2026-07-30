# simplearchive

I love ArchiveBox, but I'm a bit frustrated by how slow it is, the difficulty in debugging, and how complicated the 0.9 beta release is.  I'm a single user who wants to archive content I like or want to find one day.  I'm running ArchiveBox on a cheap VPS and it's too demanding!

I also want the archiving steps to be configurable!

# Decisions

Compatible with ArchiveBox's file format, reusing their 0.7/0.8 directory structure and index.json, allowing things to be reimported into ArchiveBox if I give up on this project:
* archive/{timestamp}
* index.json

Backend in `go` for small binary footprint that also supports building web servers very well.
* `chi/echo` HTTP server
* slog for structured logging, no metrics or traces for now

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
