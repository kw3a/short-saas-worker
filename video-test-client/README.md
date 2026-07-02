# video-test-client

A minimal Astro app for exercising **go-video** directly over HTTP, without
going through the Next.js app's session auth, credit balance checks, or its
own database writes (`balance`, `narratedVideo`, `askredditVideo`, etc.).

It has two forms — Narration and AskReddit — that POST straight to go-video's
`/generation/narration` and `/generation/askreddit` endpoints with the shared
bearer secret, then poll go-video's own Postgres for the `video.status` and
`video.progress` columns go-video's worker updates (the latter tracks real
ffmpeg encode progress, not just coarse stage checkpoints — see
`../go-video/internal/render/pipeline.go`'s `progressReporter`), showing a
live progress bar until the render finishes, and preview the finished render
straight from R2/MinIO once it's done.

This only replaces the web app's layer. go-video itself still needs its own
Postgres (for status) and R2-compatible bucket (for uploads) to run — for
local testing, that's the `db` and `minio` services in `../go-video/docker-compose.yml`.

## Run it

1. Start go-video's own dependencies and the server (see `../go-video/README.md`):
   ```bash
   cd ../go-video
   docker compose up -d db minio minio-init
   go run ./cmd/server
   ```
2. Configure this client:
   ```bash
   cp .env.example .env
   # VIDEO_SERVER_SECRET must match ../go-video/.env's VIDEO_SERVER_SECRET.
   # DATABASE_URL/R2_* should point at the same db/minio containers above.
   ```
3. Install and run:
   ```bash
   npm install
   npm run dev
   ```
4. Open http://localhost:4790, pick Narration or AskReddit, submit, and watch
   the status line update until the render finishes (or fails).

## Why it inserts a DB row

go-video's status/progress updates are plain `UPDATE video SET ... WHERE id=$1`
(`../go-video/internal/store/store.go`) — a no-op if no row with that id
exists yet. In production the Next.js app inserts that row before calling
go-video. This client does the same (`src/lib/db.ts`), inserting only the bare
`video` row go-video's own schema needs (id, a fixed `user_id`, `type`,
`status`, `progress`) — nothing from the app's credits/user tables.

`video.progress` is a column this project added to go-video's own schema
(`../go-video/db/schema.sql`) — it's not yet on the Next.js app's Drizzle
schema, so a real migration is needed there before relying on it against the
shared production DB (see the note in that schema file).
