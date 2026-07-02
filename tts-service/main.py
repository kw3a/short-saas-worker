"""TTS microservice for go-video.

Wraps Azure Speech (word-level timestamps) behind a tiny HTTP contract that the
Go renderer calls. Kept deliberately small: one endpoint, bearer auth, returns
the synthesized WAV (base64) plus the per-word boundary timings.

Contract (must match internal/tts/client.go in go-video):

    POST /synthesize   (Authorization: Bearer $TTS_SECRET)
      request : { "text": "...", "voice": "en-US-BrianNeural" }
      response: { "timestamps": [ { "word","start","duration" }, ... ],
                  "audio_b64": "<base64 wav>" }
"""

import base64
import os
import tempfile

from fastapi import FastAPI, Header, HTTPException
from pydantic import BaseModel, field_validator

from tts_azure import TTSAzure

if os.getenv("ENVIRONMENT") != "production":
    from dotenv import load_dotenv

    load_dotenv()

app = FastAPI(title="tts-service")

# Mirror the voice allow-list from the Go/Python video servers so the TTS service
# never gets asked to synthesize an unexpected voice.
VALID_VOICES = {
    "en-US-BrianNeural",
    "en-US-AvaNeural",
    "en-US-AndrewNeural",
    "en-US-EmmaNeural",
    "en-US-JennyNeural",
    "es-BO-SofiaNeural",
    "es-BO-MarceloNeural",
    "es-MX-JorgeNeural",
    "es-MX-DaliaNeural",
    "es-DO-EmilioNeural",
}


class SynthRequest(BaseModel):
    text: str
    voice: str

    @field_validator("text")
    @classmethod
    def validate_text(cls, v: str) -> str:
        v = v.strip()
        if not (1 <= len(v) <= 2000):
            raise ValueError("text must be between 1 and 2000 characters")
        return v

    @field_validator("voice")
    @classmethod
    def validate_voice(cls, v: str) -> str:
        v = v.strip()
        if v not in VALID_VOICES:
            raise ValueError("voice is not valid")
        return v


def _verify_auth(authorization: str | None) -> None:
    secret = os.environ.get("TTS_SECRET") or os.environ.get("VIDEO_SERVER_SECRET")
    if not secret:
        raise HTTPException(status_code=500, detail="Server not configured")
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(status_code=401, detail="Unauthorized")
    token = authorization.split(" ", 1)[1].strip()
    if token != secret:
        raise HTTPException(status_code=401, detail="Unauthorized")


@app.get("/health")
def health():
    return {"status": "ok"}


@app.post("/synthesize")
def synthesize(
    request: SynthRequest,
    authorization: str | None = Header(default=None, convert_underscores=False),
):
    _verify_auth(authorization)

    with tempfile.NamedTemporaryFile(suffix=".wav", delete=True) as tmp:
        timestamps = TTSAzure(request.text, request.voice, tmp.name)
        tmp.seek(0)
        audio_bytes = tmp.read()

    if not audio_bytes:
        raise HTTPException(status_code=502, detail="TTS produced no audio")

    return {
        "timestamps": timestamps,
        "audio_b64": base64.b64encode(audio_bytes).decode("ascii"),
    }
