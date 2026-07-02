# ---- build stage ----
FROM golang:1.26-bookworm AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Pure static binary (no cgo) — the Azure Speech SDK lives in the separate
# Python TTS service, so nothing here needs native linking.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/go-video ./cmd/server

# ---- runtime stage ----
# debian-slim because the renderer shells out to ffmpeg and needs DejaVu fonts.
# The Go binary itself is static; ffmpeg + fonts are the only runtime deps.
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ffmpeg \
       fonts-dejavu-core \
       ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /out/go-video /app/go-video
COPY assets /app/assets

# ASSETS_DIR must contain Montserrat-ExtraBold.ttf (baked in assets/) plus the
# backgrounds/ and musics/ directories — mount those at runtime, e.g.:
#   -v /data/backgrounds:/app/assets/backgrounds -v /data/musics:/app/assets/musics
ENV ASSETS_DIR=/app/assets
ENV PORT=3000
EXPOSE 3000

ENTRYPOINT ["/app/go-video"]
