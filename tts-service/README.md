# tts-service

Tiny HTTP wrapper around **Azure Speech** that the [`go-video`](../go-video)
renderer calls for text-to-speech with word-level timestamps. It exists so the Go
binary can stay pure-static (`CGO_ENABLED=0`) instead of linking the Azure SDK.

## Contract

This must stay in sync with `go-video/internal/tts/client.go`.

```
POST /synthesize   (Authorization: Bearer $TTS_SECRET)
  request : { "text": "...", "voice": "en-US-BrianNeural" }
  response: { "timestamps": [ { "word", "start", "duration" }, ... ],
              "audio_b64": "<base64 wav>" }

GET /health -> { "status": "ok" }
```

- `start` / `duration` are in seconds. Timestamps are filtered to word
  boundaries (no punctuation), matching the original `tts_azure.py`.
- `audio_b64` is the base64 of a RIFF WAV (Azure's default mono PCM).

## Run

```bash
python -m venv .venv && . .venv/bin/activate
pip install -r requirements.txt
cp .env.example .env          # fill AZURE_KEY, AZURE_REGION, TTS_SECRET
uvicorn main:app --port 3001
```

Point `go-video` at it with `TTS_BASE_URL=http://localhost:3001` and a matching
`TTS_SECRET`.

## Docker

```bash
docker build -t tts-service .
docker run --rm -p 3001:3001 --env-file .env tts-service
```

## Smoke test

```bash
curl -sS localhost:3001/synthesize \
  -H "Authorization: Bearer $TTS_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"text":"this is a test","voice":"en-US-BrianNeural"}' \
  | python -c 'import sys,json,base64; d=json.load(sys.stdin); \
      print("words:", d["timestamps"]); \
      print("wav bytes:", len(base64.b64decode(d["audio_b64"])))'
```
