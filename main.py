from fastapi import FastAPI, HTTPException, Header
from pydantic import BaseModel, validator
from typing import Optional
from uuid import UUID
from multiprocessing import Pool
import os
from pathlib import Path
import tempfile

from db import _db_update_video_status
from r2 import upload_to_S3, upload_thumbnail_to_S3
from tts_azure import TTSAzure
from video_edition import buildClip, add_subtitles_to_video
from comment_screenshot import generate_reddit_title_screenshot, generate_reddit_comment
from moviepy import AudioFileClip, concatenate_audioclips, VideoFileClip, ImageClip, concatenate_videoclips, CompositeVideoClip
from PIL import Image as PILImage
from moviepy.audio.AudioClip import CompositeAudioClip
from moviepy.audio.fx import AudioLoop, MultiplyVolume
import numpy as np

from dotenv import load_dotenv

if os.getenv("ENVIRONMENT") != "production":
    load_dotenv()

app = FastAPI()

pool = None  # will be initialized at startup

BACKGROUND_MUSICS_DIR = "musics"
BACKGROUND_VIDEOS_DIR = "backgrounds"
OUTPUT_DIR = "outputs"

VALID_BG_VIDEOS = ["gtav", "minecraft", "roblox", "subways", "satisfying"]
VALID_VOICES = [
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
]
VALID_MUSICS = [
    "elevator",
    "else",
    "hiddenagenda",
    "nocturne",
    "sneakysnitch",
    "tiptoes",
    "wiener",
    "waltz",
]

class NarrationRequest(BaseModel):
    id: str
    script: str
    title: Optional[str] = None
    bg_video: str
    voice: str
    music: Optional[str] = None
    free_trial: Optional[bool] = None

    @validator("id")
    def validate_uuid(cls, v):
        v = v.strip()
        try:
            UUID(v)
        except ValueError:
            raise ValueError("id must be a valid UUID")
        return v

    @validator("script")
    def validate_script(cls, v):
        if not isinstance(v, str):
            raise ValueError("script must be a string")
        v2 = v.strip()
        if not (1 <= len(v2) <= 2000):
            raise ValueError("script must be between 1 and 2000 characters")
        return v2

    @validator("title")
    def validate_title(cls, v):
        if v is None:
            return v
        v2 = v.strip()
        if len(v2) > 100:
            raise ValueError("title must be at most 100 characters")
        return v2

    @validator("bg_video")
    def validate_bg_video(cls, v):
        v = v.strip()
        if v not in VALID_BG_VIDEOS:
            raise ValueError(f"bg_video must be one of {VALID_BG_VIDEOS}")
        return v

    @validator("voice")
    def validate_voice(cls, v):
        v = v.strip()
        if v not in VALID_VOICES:
            raise ValueError(f"voice must be one of {VALID_VOICES}")
        return v

    @validator("music")
    def validate_music(cls, v):
        if v:
            v = v.strip()
            if v not in VALID_MUSICS:
                raise ValueError(f"music must be one of {VALID_MUSICS}")
        return v

    @validator("free_trial")
    def validate_free_trial(cls, v):
        if v is None:
            return v
        if not isinstance(v, bool):
            raise ValueError("free_trial must be a boolean")
        return v

class AskRedditRequest(BaseModel):
    id: str
    title: str
    comments: list[str]
    bg_video: str
    voice: str
    music: Optional[str] = None
    free_trial: Optional[bool] = None

    @validator("id")
    def validate_uuid(cls, v):
        v = v.strip()
        try:
            UUID(v)
        except ValueError:
            raise ValueError("id must be a valid UUID")
        return v

    @validator("title")
    def validate_title(cls, v):
        v = v.strip()
        if not (1 <= len(v) <= 100):
            raise ValueError("title must be between 1 and 100 characters")
        return v

    @validator("comments")
    def validate_comments(cls, v: list[str]):
        if not isinstance(v, list):
            raise ValueError("comments must be a list of strings")
        if not (1 <= len(v) <= 20):
            raise ValueError("comments count must be between 1 and 20")
        total_len = 0
        cleaned: list[str] = []
        for c in v:
            if not isinstance(c, str):
                raise ValueError("each comment must be a string")
            c2 = c.strip()
            if not (1 <= len(c2) <= 1000):
                raise ValueError("each comment length must be between 1 and 1000 characters")
            cleaned.append(c2)
            total_len += len(c2)
        if total_len >= 2000:
            raise ValueError("sum of comments length must be less than 2000 characters")
        return cleaned

    @validator("bg_video")
    def validate_bg_video(cls, v):
        v = v.strip()
        if v not in VALID_BG_VIDEOS:
            raise ValueError(f"bg_video must be one of {VALID_BG_VIDEOS}")
        return v

    @validator("voice")
    def validate_voice(cls, v):
        v = v.strip()
        if v not in VALID_VOICES:
            raise ValueError(f"voice must be one of {VALID_VOICES}")
        return v

    @validator("music")
    def validate_music(cls, v):
        if v:
            v = v.strip()
            if v not in VALID_MUSICS:
                raise ValueError(f"music must be one of {VALID_MUSICS}")
        return v

    
    @validator("free_trial")
    def validate_free_trial(cls, v):
        if v is None:
            return v
        if not isinstance(v, bool):
            raise ValueError("free_trial must be a boolean")
        return v

def narration_gen_task(id: str, script: str, title: str | None, bg_video: str, voice: str, music: str | None, free_trial: bool | None):
    print(f"[{id}] Task started with title: {title}, bg_video: {bg_video}, voice: {voice}, music: {music}")
    try:
        # 1) Mark as rendering
        _db_update_video_status(id, "rendering")

        # 2) Resolve paths (use temp dir for intermediates)
        root_dir = Path(__file__).parent
        with tempfile.TemporaryDirectory(prefix=f"vid_{id}_") as tmpdir:
            outputs_root = Path(tmpdir)

        # Background video must exist exactly as backgrounds/<bg_video>
            bg_file = root_dir / BACKGROUND_VIDEOS_DIR / (bg_video + ".mp4")
            if not bg_file.is_file():
                raise RuntimeError("Background video not found")
            bg_path = str(bg_file)

        # 3) Generate TTS for title (if present) and script, then concatenate
            title_duration = 0.0
            title_audio_path = None
            if title and title.strip():
                title_audio_path = str(outputs_root / "title.wav")
                _ = TTSAzure(title.strip(), voice, title_audio_path)
                title_audio_clip = AudioFileClip(title_audio_path)
                title_duration = float(title_audio_clip.duration)
            script_audio_path = str(outputs_root / "script.wav")
            subs_script = TTSAzure(script, voice, script_audio_path)

        # Concatenate audio to single narration
            audio_parts = []
            if title_audio_path:
                audio_parts.append(AudioFileClip(title_audio_path))
            audio_parts.append(AudioFileClip(script_audio_path))
            combined_audio = concatenate_audioclips(audio_parts) if len(audio_parts) > 1 else audio_parts[0]
            narration_path = str(outputs_root / "narration.wav")

        # Optional background music: loop to match duration and lower volume
            mixed_audio = _mix_bg_music(combined_audio, float(combined_audio.duration), music, root_dir, id)
            mixed_audio.write_audiofile(narration_path, fps=44100)

        # Build combined subtitles with title card first (entire title at once), then word-by-word script
            subs: list[dict] = []
            if title and title.strip():
                subs.append({
                    'word': title.strip(),
                    'start': 0.0,
                    'duration': title_duration,
                    'is_title': True,
                })
            for s in (subs_script or []):
                subs.append({
                    'word': s.get('word', ''),
                    'start': float(s.get('start', 0.0)) + title_duration,
                    'duration': float(s.get('duration', 0.0)),
                })

            # 4) Build 9:16 clip and add subtitles
            clip = buildClip(bg_path, narration_path)
            out_path = str(outputs_root / f"{id}.mp4")
            add_subtitles_to_video(clip, subs or [], out_path)

            # Apply watermark for free trial
            if free_trial:
                try:
                    with VideoFileClip(out_path) as base_v:
                        wm_clip = _make_watermark_clip(base_v.w, base_v.h)
                        wm_clip = wm_clip.with_duration(base_v.duration).with_position(( (base_v.w - wm_clip.w)//2, base_v.h - wm_clip.h - 24 ))
                        watermarked = CompositeVideoClip([base_v, wm_clip]).with_duration(base_v.duration)
                        out_wm = str(outputs_root / f"{id}_wm.mp4")
                        watermarked.write_videofile(out_wm, fps=30, audio_codec="aac")
                        out_path = out_wm
                except Exception as e:
                    print(f"[{id}] Failed to apply watermark: {e}. Proceeding without watermark.")

        # 5) Upload to R2 with key <id>.mp4
            upload_to_S3(out_path, id)

        # 5.1) Create and upload thumbnail from first frame
            thumb_path = str(outputs_root / f"{id}.jpg")
            try:
                with VideoFileClip(out_path) as vclip:
                    vclip.save_frame(thumb_path, t=0.0)
                upload_thumbnail_to_S3(thumb_path, id)
            except Exception:
                pass

        # 6) Mark completed
            _db_update_video_status(id, "completed")
            print(f"[{id}] Task completed successfully")
    except Exception as e:
        _db_update_video_status(id, "failed")
        print(f"[{id}] Task failed with error: {e}")

@app.on_event("startup")
def startup_event():
    global pool
    print("Starting process pool...")
    pool = Pool(processes=2)
    print("Pool initialized.")

def _mix_bg_music(base_audio, duration: float, music: str | None, root_dir: Path, id: str):
    """Return a CompositeAudioClip with quiet background music mixed under base_audio.
    If music is None or file missing, returns base_audio unchanged."""
    if not music:
        return base_audio
    music_file = root_dir / BACKGROUND_MUSICS_DIR / (music + ".mp3")
    if not music_file.is_file():
        print(f"[{id}] Background music file not found: {music_file}. Skipping music.")
        return base_audio
    try:
        bg_music_clip = AudioFileClip(str(music_file))
        #looped = AudioLoop(bg_music_clip, duration=duration)
        #quiet = looped.with_effects([afx.MultiplyVolume(0.25)])
        quiet = bg_music_clip.with_effects([MultiplyVolume(0.25), AudioLoop(duration=duration)])
        return CompositeAudioClip([base_audio, quiet])
    except Exception as e:
        print(f"[{id}] Failed mixing bg music: {e}. Proceeding without music.")
        return base_audio

def _make_watermark_clip(video_w: int, video_h: int) -> ImageClip:
    """Create a semi-opaque dark banner watermark with a left play triangle and text 'viralshort.app'.
    The banner width tightly fits its contents with equal left/right padding to avoid wasted space."""
    # Height relative to video, width computed from content
    banner_h = max(40, int(video_h * 0.07))
    padding_x = max(12, int(banner_h * 0.25))
    gap = max(8, int(banner_h * 0.2))
    # Slightly more transparent background
    alpha = int(0.6 * 255)

    # Prepare font and measure text first
    try:
        from PIL import ImageFont, ImageDraw
        # Temporary canvas for measurement
        tmp_img = PILImage.new("RGBA", (10, 10))
        tmp_draw = ImageDraw.Draw(tmp_img)
        try:
            font_size = max(16, int(banner_h * 0.45))
            font = ImageFont.truetype("DejaVuSans.ttf", font_size)
        except Exception:
            font = ImageFont.load_default()
        text = "viralshort.app"
        try:
            bbox = tmp_draw.textbbox((0, 0), text, font=font)
            tw = bbox[2] - bbox[0]
            th = bbox[3] - bbox[1]
        except Exception:
            tw, th = tmp_draw.textsize(text, font=font)

        # Triangle (play icon) dimensions
        tri_h = int(banner_h * 0.5)
        tri_w = int(tri_h * 0.6)

        # Compute tight banner width from content
        content_w = padding_x + tri_w + gap + tw + padding_x
        max_w = int(video_w * 0.9)
        banner_w = min(max(240, content_w), max_w)

        # Create final banner and draw
        img = PILImage.new("RGBA", (banner_w, banner_h), (0, 0, 0, alpha))
        draw = ImageDraw.Draw(img)

        cy = banner_h // 2
        x0 = padding_x
        triangle = [(x0, cy - tri_h // 2), (x0, cy + tri_h // 2), (x0 + tri_w, cy)]
        # Hollow triangle (outline only)
        try:
            draw.polygon(triangle, outline=(255, 255, 255, 255), fill=None, width=max(2, int(banner_h * 0.08)))
        except TypeError:
            # Older Pillow may not support width on polygon; draw edges manually
            lw = max(2, int(banner_h * 0.08))
            draw.line([triangle[0], triangle[1]], fill=(255, 255, 255, 255), width=lw)
            draw.line([triangle[1], triangle[2]], fill=(255, 255, 255, 255), width=lw)
            draw.line([triangle[2], triangle[0]], fill=(255, 255, 255, 255), width=lw)

        tx = x0 + tri_w + gap
        ty = max(0, (banner_h - th) // 2)
        draw.text((tx, ty), text, font=font, fill=(255, 255, 255, 255))
    except Exception:
        # Fallback: simple fixed-size banner if anything fails
        banner_w = max(240, int(video_w * 0.5))
        img = PILImage.new("RGBA", (banner_w, banner_h), (0, 0, 0, alpha))

    arr = np.array(img)
    clip = ImageClip(arr)
    return clip

@app.on_event("shutdown")
def shutdown_event():
    global pool
    if pool:
        print("Shutting down pool...")
        pool.terminate()
        #pool.close()
        pool.join()
        print("Pool shut down cleanly.")

def _verify_auth(authorization: str | None):
    secret = os.environ.get("VIDEO_SERVER_SECRET")
    if not secret:
        raise HTTPException(status_code=500, detail="Server not configured")
    if not authorization or not authorization.startswith("Bearer "):
        raise HTTPException(status_code=401, detail="Unauthorized")
    token = authorization.split(" ", 1)[1].strip()
    if token != secret:
        raise HTTPException(status_code=401, detail="Unauthorized")

def askreddit_gen_task(id: str, title: str | None, comments: list[str], bg_video: str, voice: str, music: str | None, free_trial: bool | None):
    print(f"[{id}] AskReddit Task started with bg_video: {bg_video}, voice: {voice}, music: {music}")
    try:
        _db_update_video_status(id, "rendering")
        root_dir = Path(__file__).parent
        with tempfile.TemporaryDirectory(prefix=f"ask_{id}_") as tmpdir:
            outputs_root = Path(tmpdir)

            # Background video path
            bg_file = root_dir / BACKGROUND_VIDEOS_DIR / (bg_video + ".mp4")
            if not bg_file.is_file():
                raise RuntimeError("Background video not found")
            bg_path = str(bg_file)

            segment_audios: list[AudioFileClip] = []
            segment_videos: list = []

            def make_segment(text: str, is_title: bool, idx: int):
                # Audio
                audio_path = str(outputs_root / f"seg_{idx}.wav")
                _ = TTSAzure(text, voice, audio_path)
                audio_clip = AudioFileClip(audio_path)

                # Screenshot image
                if is_title:
                    shot_path = str(outputs_root / f"title_{idx}.png")
                    generate_reddit_title_screenshot("r/AskReddit", text, output_path=shot_path)
                else:
                    shot_path = str(outputs_root / f"comment_{idx}.png")
                    generate_reddit_comment(text, output_path=shot_path)

                # Build background clip using provided bg_video and segment audio (fits to 9:16)
                base_clip = buildClip(bg_path, audio_path)

                # Foreground image centered, scaled to fit within background without overflow
                with PILImage.open(shot_path) as im:
                    img_w, img_h = im.size
                max_w = int(base_clip.w * 0.88)
                max_h = int(base_clip.h * 0.78)
                scale = min(max_w / img_w, max_h / img_h, 1.0)
                new_w = max(1, int(img_w * scale))
                new_h = max(1, int(img_h * scale))
                try:
                    from PIL import Image as _PILImage
                    resample = _PILImage.Resampling.LANCZOS
                except Exception:
                    resample = PILImage.LANCZOS if hasattr(PILImage, 'LANCZOS') else PILImage.BICUBIC
                resized = PILImage.open(shot_path).resize((new_w, new_h), resample=resample)
                resized_path = str(outputs_root / f"{Path(shot_path).stem}_rs.png")
                resized.save(resized_path)

                img_clip = (
                    ImageClip(resized_path)
                    .with_duration(float(audio_clip.duration))
                    .with_position("center")
                    .with_opacity(0.90)
                )
                comp = CompositeVideoClip([base_clip, img_clip]).with_duration(float(audio_clip.duration))
                comp = comp.with_audio(audio_clip)
                return audio_clip, comp

            idx = 0
            if title and title.strip():
                a, v = make_segment(title.strip(), True, idx)
                segment_audios.append(a)
                segment_videos.append(v)
                idx += 1

            for c in comments:
                c2 = c.strip()
                if not c2:
                    continue
                a, v = make_segment(c2, False, idx)
                segment_audios.append(a)
                segment_videos.append(v)
                idx += 1

            if not segment_videos:
                raise RuntimeError("No segments to build")

            final_clip = concatenate_videoclips(segment_videos, method="compose")

            # Optional background music (shared helper)
            mixed_audio = _mix_bg_music(final_clip.audio, float(final_clip.duration), music, root_dir, id)
            final_clip = final_clip.with_audio(mixed_audio)

            # Apply watermark for free trial
            if free_trial:
                try:
                    wm_clip = _make_watermark_clip(final_clip.w, final_clip.h).with_duration(final_clip.duration)
                    wm_clip = wm_clip.with_position(((final_clip.w - wm_clip.w)//2, final_clip.h - wm_clip.h - 24))
                    final_clip = CompositeVideoClip([final_clip, wm_clip]).with_duration(final_clip.duration)
                except Exception as e:
                    print(f"[{id}] Failed to apply watermark: {e}. Proceeding without watermark.")

            out_path = str(outputs_root / f"{id}.mp4")
            final_clip.write_videofile(out_path, fps=30, audio_codec="aac")

            upload_to_S3(out_path, id)

            thumb_path = str(outputs_root / f"{id}.jpg")
            try:
                with VideoFileClip(out_path) as vclip:
                    vclip.save_frame(thumb_path, t=0.0)
                upload_thumbnail_to_S3(thumb_path, id)
            except Exception:
                pass

            _db_update_video_status(id, "completed")
            print(f"[{id}] AskReddit Task completed successfully")
    except Exception as e:
        _db_update_video_status(id, "failed")
        print(f"[{id}] AskReddit Task failed with error: {e}")

@app.post("/generation/narration")
def generate_narration(request: NarrationRequest, authorization: str | None = Header(default=None, convert_underscores=False)):
    _verify_auth(authorization)
    print(f"[{request.id}] Task enqueued")
    global pool
    if not pool:
        raise HTTPException(status_code=500, detail="Process pool not initialized")

    pool.apply_async(
        narration_gen_task,
        args=(request.id, request.script, request.title, request.bg_video, request.voice, request.music, request.free_trial)
    )
    return {"status": "Ok"}

@app.post("/generation/askreddit")
def generate_askreddit(request: AskRedditRequest, authorization: str | None = Header(default=None, convert_underscores=False)):
    """
    Validate AskReddit generation payload. Authentication and field validations only.
    No background processing here yet.
    """
    _verify_auth(authorization)
    print(f"[{request.id}] AskReddit enqueued")
    global pool
    if not pool:
        raise HTTPException(status_code=500, detail="Process pool not initialized")
    pool.apply_async(
        askreddit_gen_task,
        args=(request.id, request.title, request.comments, request.bg_video, request.voice, request.music, request.free_trial)
    )
    return {"status": "Ok"}
