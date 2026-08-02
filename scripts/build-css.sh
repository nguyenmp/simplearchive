#!/bin/sh
# build-css.sh — compile the tailwind stylesheet for the server's embedded
# static assets. Build the dev image first if you don't have it:
#
#   make dev-image
#
# Then run from the repo root inside the dev container:
#
#   docker run --rm -v "$PWD:/app" -w /app simplearchive-dev make css
#
# The generated tailwind.css is committed so the repo builds offline; re-run
# this whenever templates change their class names.
set -eu

INPUT="internal/server/assets/input.css"
OUTPUT="internal/server/assets/static/tailwind.css"

mkdir -p "$(dirname "$OUTPUT")"
tailwindcss -i "$INPUT" -o "$OUTPUT" --minify
echo "wrote $OUTPUT"
