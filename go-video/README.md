# go-video

Go rewrite of the video-generation server. It renders 9:16 short videos
(narration + AskReddit styles) by orchestrating **ffmpeg** directly — no MoviePy.
Text-to-speech (Azure Speech, with word-level timestamps) is delegated to a small
separate **Python TTS microservice**, so this binary stays pure-static
(`CGO_ENABLED=0`).

## Architecture

```
Next.js app ──HTTP──▶ go-video (this)  ──HTTP──▶ tts-service (Python/Azure)
                          │  ├─ ffmpeg/ffprobe (crop, overlay, mix, encode)
                          │  ├─ gg (subtitle cards, title cards, reddit shots, watermark)
                          │  ├─ Postgres (sqlc) — video.status/progress updates
                          │  └─ R2 (aws-sdk-go-v2) — upload <id>.mp4 + <id>.jpg
```

- **chi** for routing, **sqlc** (pgx/v5) for DB access.
- In-process worker pool (`internal/queue`) runs renders off the request path.
  Swap for Redis/BullMQ or a DB-backed queue for durability — the `Submit/Job`
  boundary is designed to localize that change.
- One ffmpeg pass per video (crop + timed overlays + audio mix + encode),
  versus MoviePy's multiple decode/re-encode passes.

## Endpoints

Both require `Authorization: Bearer $VIDEO_SERVER_SECRET` and return
`{"status":"Ok"}` on enqueue (validation mirrors the old FastAPI service).

- `POST /generation/narration` — `{ id, script, title?, bg_video, voice, music?, free_trial? }`
- `POST /generation/askreddit` — `{ id, title, comments[], bg_video, voice, music?, free_trial? }`
- `GET /health`

There's no HTTP status/progress endpoint — callers poll the `video` table directly
(same as the Next.js app already does). Alongside `status`
(`queued`/`rendering`/`completed`/`failed`), the worker also writes `progress`
(0-100): fixed checkpoints for TTS/subtitle-rendering/upload stages, plus real
per-frame progress during the ffmpeg encode step(s) — the dominant cost — parsed
from `ffmpeg -progress`. See `internal/render/pipeline.go`'s `progressReporter`.

## The TTS service contract (companion Python service)

This binary calls the TTS service, implemented in [`../tts-service`](../tts-service)
(FastAPI wrapper around Azure Speech):

```
POST /synthesize   (Authorization: Bearer $TTS_SECRET)
  request : { "text": "...", "voice": "en-US-BrianNeural" }
  response: { "timestamps": [ { "word","start","duration" }, ... ],
              "audio_b64": "<base64 wav>" }
```

## Configuration

See `.env.example`. `ASSETS_DIR` must contain `Montserrat-ExtraBold.ttf`
(baked into `assets/`) plus `backgrounds/<name>.mp4` and `musics/<name>.mp3`.

Set `R2_ENDPOINT` to point uploads at a local S3-compatible mock (MinIO, via
`docker compose up -d minio minio-init`) instead of real Cloudflare R2 — see
Develop below. Leave it empty to upload to the real R2 endpoint derived from
`R2_ACCOUNT_ID`.

## Develop

```bash
docker compose up -d db      # local Postgres, auto-loads db/schema.sql
docker compose up -d minio minio-init  # local R2 mock (MinIO) + bucket creation
go run ./cmd/server          # needs a .env (see .env.example)
go build ./...               # compile
go vet ./...                 # static checks
sqlc generate                # regenerate db/sqlc after editing db/query.sql
```

## Build

```bash
docker build -t go-video .
docker run --rm -p 3000:3000 --env-file .env \
  -v /data/backgrounds:/app/assets/backgrounds \
  -v /data/musics:/app/assets/musics \
  go-video
```

## Status / parity notes

- Visual output is faithful to the Pillow original but **not pixel-identical**
  (gg/freetype antialiasing differs slightly from Pillow).
- Subtitles use timed PNG overlays (`overlay=...:enable='between(t,a,b)'`); a long
  script produces many overlay nodes in one filtergraph — fine, but a candidate
  for batching/ASS if it ever gets heavy.
- Length limits are validated in Unicode characters (runes), matching Pydantic;
  unknown JSON fields are ignored, also matching Pydantic.
- The Python TTS service now lives in [`../tts-service`](../tts-service). Still
  not wired: a durable queue (the in-process pool loses jobs on restart).
- `video.progress` (`db/schema.sql`) is additive and not yet on the Next.js
  app's Drizzle schema (`app/src/db/schema.ts`) — add the column there via a
  migration before relying on it against the shared production DB.
