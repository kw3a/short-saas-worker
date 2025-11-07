import os
import numpy as np
from PIL import Image, ImageDraw, ImageFont
from moviepy import VideoFileClip, AudioFileClip, TextClip, CompositeVideoClip, ImageClip
import random

def crop_to_9_16(video):
    original_width, original_height = video.size
    new_width = original_height * 9 // 16

    x_center = original_width // 2
    x1 = max(0, x_center - new_width // 2)
    x2 = min(original_width, x_center + new_width // 2)

    cropped_video = video.cropped(x1=x1, y1=0, x2=x2, y2=original_height)
    return cropped_video

def buildClip(video_file, audio_file):
    video = VideoFileClip(video_file)
    audio = AudioFileClip(audio_file)
    time_delta = 0.5
    audio_duration = audio.duration + time_delta
    video_duration = video.duration

    superior_limit = video_duration - audio_duration 
    start = random.uniform(0, superior_limit)
    end = start + audio_duration 
    clip = video.subclipped(start, end)
    clip = crop_to_9_16(clip)
    clip = clip.with_audio(audio)
    return clip

def add_subtitles_to_video(video, subtitles, output_file):
    text_clips = []
    bg_clips = []
    font_path = "Montserrat-ExtraBold.ttf"
    fontsize = 56
    stroke_w = 4

    def wrap_long_word_to_width(word: str, max_px_width: int) -> str:
        # Returns possibly multiline text with hyphenation to not exceed max_px_width
        if not word:
            return word
        # Load font once per call (kept small)
        try:
            font = ImageFont.truetype(font_path, fontsize)
        except Exception:
            font = ImageFont.load_default()
        # Quick accept: if whole word fits, return as-is
        dummy = Image.new('RGBA', (10, 10))
        draw = ImageDraw.Draw(dummy)
        bbox = draw.textbbox((0, 0), word, font=font, stroke_width=stroke_w)
        if bbox[2] - bbox[0] <= max_px_width:
            return word
        # Otherwise, greedily split into lines with hyphenation
        lines = []
        start = 0
        n = len(word)
        while start < n:
            # Grow end until width exceeds
            end = start + 1
            last_fit_end = start
            while end <= n:
                candidate = word[start:end]
                # add hyphen when there is remaining text
                candidate_draw = candidate + ("-" if end < n else "")
                bbox = draw.textbbox((0, 0), candidate_draw, font=font, stroke_width=stroke_w)
                width = bbox[2] - bbox[0]
                if width <= max_px_width:
                    last_fit_end = end
                    end += 1
                else:
                    break
            if last_fit_end == start:
                # Fallback: force at least 1 char with hyphen
                last_fit_end = min(start + 1, n)
            # Append line, with hyphen if not the last slice
            line = word[start:last_fit_end]
            if last_fit_end < n:
                line = line + "-"
            lines.append(line)
            start = last_fit_end
        return "\n".join(lines)
    
    for subtitle in subtitles:
        is_title = bool(subtitle.get('is_title'))
        raw_text = subtitle['word']
        start_time = subtitle['start']
        duration = subtitle['duration']
        if raw_text != "":
            if is_title:
                title_text = raw_text.upper()
                # Constrain title within 80% width and 55% height of the frame
                max_w = int(video.w * 0.80)
                max_h = int(video.h * 0.55)
                pad_x, pad_y = 20, 16
                radius = 20

                def make_text(fs: int):
                    return TextClip(
                        text=title_text,
                        font_size=fs,
                        font=font_path,
                        color='black',
                        method='caption',
                        size=(max_w, None),
                    )

                fs = fontsize
                text_clip = make_text(fs)
                # Reduce font size until it fits max_h with padding
                while (text_clip.h + 2 * pad_y) > max_h and fs > 24:
                    fs = max(24, int(fs * 0.92))
                    text_clip = make_text(fs)

                text_clip = text_clip.with_duration(duration).with_start(start_time).with_position('center').with_opacity(0.75)

                bg_w, bg_h = text_clip.w + 2 * pad_x, text_clip.h + 2 * pad_y

                back_img = Image.new('RGBA', (bg_w, bg_h), (0, 0, 0, 0))
                back_draw = ImageDraw.Draw(back_img)
                back_fill = (255, 165, 0, 192)  # ~75% transparent orange
                back_draw.rounded_rectangle([(0, 0), (bg_w - 1, bg_h - 1)], radius=radius, fill=back_fill)

                front_img = Image.new('RGBA', (bg_w, bg_h), (0, 0, 0, 0))
                front_draw = ImageDraw.Draw(front_img)
                front_fill = (255, 255, 255, 192)  # ~75% transparent white 
                front_draw.rounded_rectangle([(0, 0), (bg_w - 1, bg_h - 1)], radius=radius, fill=front_fill)

                back_clip = ImageClip(np.array(back_img)).with_duration(duration).with_start(start_time).rotated(-6).with_position('center')
                front_clip = ImageClip(np.array(front_img)).with_duration(duration).with_start(start_time).rotated(2).with_position('center')

                bg_clips.append(back_clip)
                bg_clips.append(front_clip)
                text_clips.append(text_clip)
            else:
                # Script word styling: uppercase, stroke, orange card per-word
                text = raw_text.upper()
                # Compute max width and hyphen-wrap long single words
                max_text_width = int(video.w * 0.9)
                wrapped_text = wrap_long_word_to_width(text, max_text_width)

                text_clip = TextClip(
                    text=wrapped_text,
                    font_size=fontsize,
                    font=font_path,
                    color='white',
                    stroke_color='black',
                    stroke_width=stroke_w,
                    method='label'
                ).with_duration(duration).with_start(start_time).with_position('center')

                # Orange rounded card for script words
                pad_x, pad_y = 10, 6
                radius = 14
                bg_w, bg_h = text_clip.w + 2 * pad_x, text_clip.h + 2 * pad_y
                img = Image.new('RGBA', (bg_w, bg_h), (0, 0, 0, 0))
                draw = ImageDraw.Draw(img)
                fill = (255, 90, 60, 230)
                draw.rounded_rectangle([(0, 0), (bg_w - 1, bg_h - 1)], radius=radius, fill=fill)
                bg_array = np.array(img)
                bg_clip = ImageClip(bg_array).with_duration(duration).with_start(start_time).with_position('center')

                bg_clips.append(bg_clip)
                text_clips.append(text_clip)
    
    video_with_subtitles = CompositeVideoClip([video] + bg_clips + text_clips)
    video_with_subtitles.write_videofile(output_file, codec='libx264')

