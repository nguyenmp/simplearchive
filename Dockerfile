# syntax=docker/dockerfile:1

# Multi-stage Dockerfile for simplearchive.
#
# Build targets:
#   docker build --target dev -t simplearchive-dev .
#   docker build --target runtime -t simplearchive .
#
# IMPORTANT: wget, yt-dlp, and chromium are runtime dependencies (the Go
# binary shells out to them). If you add/remove a runtime tool here, keep the
# dev and runtime stages in sync.

# Build stage — compiles the Go binary.
FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates curl git libgcc libstdc++
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -tags chromedp -o /usr/local/bin/simplearchive .

# Dev stage — local development environment.
# Includes runtime tools (must match runtime stage below) + Go toolchain + Tailwind CSS CLI.
FROM golang:1.26-alpine AS dev
RUN apk add --no-cache \
    ca-certificates curl libgcc libstdc++ wget yt-dlp chromium
RUN case "$(uname -m)" in \
        x86_64)  ARCH=x64-musl ;; \
        aarch64) ARCH=arm64-musl ;; \
        *) echo "unsupported arch: $(uname -m)"; exit 1 ;; \
    esac && \
    wget -O /usr/local/bin/tailwindcss \
        "https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.3/tailwindcss-linux-${ARCH}" && \
    chmod +x /usr/local/bin/tailwindcss
WORKDIR /app
CMD ["sh"]

# Runtime stage — minimal production image with runtime tools only.
# Keep the RUN apk add list in sync with the dev stage above.
FROM alpine:latest AS runtime
RUN apk add --no-cache \
    ca-certificates curl libgcc libstdc++ wget yt-dlp chromium
COPY --from=build /usr/local/bin/simplearchive /usr/local/bin/simplearchive
WORKDIR /data
ENV SERVE_ADDR=0.0.0.0:8080
EXPOSE 8080
CMD ["simplearchive", "serve"]
