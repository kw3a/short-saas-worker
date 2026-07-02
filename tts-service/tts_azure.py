"""Azure Speech synthesis with word-level timestamps.

Ported from video-server/tts_azure.py. Writes a WAV to `file_name` and returns a
list of {word, start, duration} dicts (seconds), filtered to word boundaries —
exactly the shape go-video's tts.Word expects.
"""

import os

import azure.cognitiveservices.speech as speechsdk


def _format_timestamp(evt: "speechsdk.SpeechSynthesisWordBoundaryEventArgs") -> dict:
    start = round(evt.audio_offset * 1e-7, 3)  # 100ns ticks -> seconds
    duration = evt.duration.total_seconds()
    return {"word": evt.text, "start": start, "duration": duration}


def TTSAzure(text: str, voice: str, file_name: str) -> list[dict]:
    speech_key = os.environ["AZURE_KEY"]
    service_region = os.environ["AZURE_REGION"]

    timestamps: list[dict] = []
    speech_config = speechsdk.SpeechConfig(subscription=speech_key, region=service_region)
    speech_config.speech_synthesis_voice_name = voice
    speech_config.request_word_level_timestamps()

    audio_config = speechsdk.audio.AudioOutputConfig(filename=file_name)
    synthesizer = speechsdk.SpeechSynthesizer(
        speech_config=speech_config, audio_config=audio_config
    )

    def _on_word(evt: "speechsdk.SpeechSynthesisWordBoundaryEventArgs") -> None:
        try:
            if evt.boundary_type == speechsdk.SpeechSynthesisBoundaryType.Word:
                timestamps.append(_format_timestamp(evt))
        except Exception:
            pass

    synthesizer.synthesis_word_boundary.connect(_on_word)

    result = synthesizer.speak_text(text)

    if result.reason == speechsdk.ResultReason.Canceled:
        details = result.cancellation_details
        msg = f"Speech synthesis canceled: {details.reason}"
        if details.reason == speechsdk.CancellationReason.Error:
            msg += f" — {details.error_details}"
        raise RuntimeError(msg)

    return timestamps
