import concurrent.futures
import azure.cognitiveservices.speech as speechsdk
import os
from uuid import uuid4


def _format_timestamp(evt: speechsdk.SpeechSynthesisWordBoundaryEventArgs):
    word = evt.text
    start = evt.audio_offset
    start = start * 1e-7
    start = round(start, 3)
    duration = evt.duration
    duration = duration.total_seconds()
    return {'word': word, 'start': start, 'duration': duration}

def TTSAzure(text: str, voice: str, file_name: str) -> list:
    speech_key = os.environ["AZURE_KEY"]
    service_region = os.environ["AZURE_REGION"]
    timestamps: list[dict] = []
    speech_config: speechsdk.SpeechConfig = speechsdk.SpeechConfig(subscription=speech_key, region=service_region)
    speech_config.speech_synthesis_voice_name = voice
    speech_config.request_word_level_timestamps()

    audio_config: speechsdk.audio.AudioOutputConfig = speechsdk.audio.AudioOutputConfig(filename=file_name)
    speech_synthesizer = speechsdk.SpeechSynthesizer(speech_config=speech_config, audio_config=audio_config)
    
    def _on_word(evt: speechsdk.SpeechSynthesisWordBoundaryEventArgs):
        try:
            if evt.boundary_type == speechsdk.SpeechSynthesisBoundaryType.Word:
                timestamps.append(_format_timestamp(evt))
        except Exception:
            pass

    speech_synthesizer.synthesis_word_boundary.connect(_on_word)

    result = speech_synthesizer.speak_text(text)

    if result.reason == speechsdk.ResultReason.SynthesizingAudioCompleted:
        print("Speech synthesis succeeded. The audio was saved to file: " + file_name)
    elif result.reason == speechsdk.ResultReason.Canceled:
        cancellation_details = result.cancellation_details
        print("Speech synthesis canceled: {}".format(cancellation_details.reason))
        if cancellation_details.reason == speechsdk.CancellationReason.Error:
            print("Error details: {}".format(cancellation_details.error_details))
    return timestamps

def tts_task(text, voice):
    # generate a unique filename for each request
    file_name = f"{uuid4()}.wav"
    subs = TTSAzure(text, voice, file_name)
    return file_name, subs

if __name__ == "__main__":
    voice = "es-MX-JorgeNeural"
    texts = [
        "Hola, este es Jorge 1.",
        "Hola, este es Jorge 2.",
        "Hola, este es Jorge 3.",
        "Hola, este es Jorge 4."
    ]

    # run 4 TTS requests in parallel
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
        futures = [executor.submit(tts_task, t, voice) for t in texts]
        for future in concurrent.futures.as_completed(futures):
            file_name, subs = future.result()
            print(f"TTS finished for {file_name}, subtitles: {subs}")
