package render

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// runFFmpeg executes ffmpeg with the given args, returning stderr on failure.
func runFFmpeg(ctx context.Context, args ...string) error {
	return runFFmpegProgress(ctx, 0, nil, args...)
}

// runFFmpegProgress is runFFmpeg, but when onProgress is non-nil and totalDur>0
// it also asks ffmpeg to report encode progress on stdout (-progress pipe:1)
// and calls onProgress with the fraction of totalDur encoded so far, roughly
// whenever ffmpeg emits a new out_time_us line (a few times per second).
func runFFmpegProgress(ctx context.Context, totalDur float64, onProgress func(frac float64), args ...string) error {
	full := []string{"-y", "-hide_banner", "-loglevel", "error"}
	tracking := onProgress != nil && totalDur > 0
	if tracking {
		full = append(full, "-progress", "pipe:1")
	}
	full = append(full, args...)

	cmd := exec.CommandContext(ctx, "ffmpeg", full...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if !tracking {
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("ffmpeg failed: %v: %s", err, stderr.String())
		}
		return nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		us, ok := strings.CutPrefix(scanner.Text(), "out_time_us=")
		if !ok {
			continue
		}
		v, err := strconv.ParseInt(us, 10, 64)
		if err != nil {
			continue
		}
		frac := float64(v) / 1e6 / totalDur
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		onProgress(frac)
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg failed: %v: %s", err, stderr.String())
	}
	return nil
}

// MediaInfo holds the probed properties needed by the pipelines.
type MediaInfo struct {
	Duration float64
	Width    int
	Height   int
}

type ffprobeOutput struct {
	Streams []struct {
		Width  int    `json:"width"`
		Height int    `json:"height"`
		CodecT string `json:"codec_type"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

// Probe returns duration and (for video) dimensions of a media file.
func Probe(ctx context.Context, path string) (MediaInfo, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration:stream=width,height,codec_type",
		"-of", "json", path,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return MediaInfo{}, fmt.Errorf("ffprobe failed for %s: %w", path, err)
	}
	var parsed ffprobeOutput
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		return MediaInfo{}, err
	}
	mi := MediaInfo{}
	mi.Duration, _ = strconv.ParseFloat(parsed.Format.Duration, 64)
	for _, s := range parsed.Streams {
		if s.CodecT == "video" && s.Width > 0 {
			mi.Width = s.Width
			mi.Height = s.Height
			break
		}
	}
	return mi, nil
}

// Thumbnail writes the first frame of videoPath to jpgPath.
func Thumbnail(ctx context.Context, videoPath, jpgPath string) error {
	return runFFmpeg(ctx, "-ss", "0", "-i", videoPath, "-frames:v", "1", "-q:v", "2", jpgPath)
}

// ConcatAudio concatenates WAV inputs into a single WAV via the concat filter.
func ConcatAudio(ctx context.Context, inputs []string, out string) error {
	if len(inputs) == 1 {
		return runFFmpeg(ctx, "-i", inputs[0], out)
	}
	args := []string{}
	for _, in := range inputs {
		args = append(args, "-i", in)
	}
	var fc bytes.Buffer
	for i := range inputs {
		fmt.Fprintf(&fc, "[%d:a]", i)
	}
	fmt.Fprintf(&fc, "concat=n=%d:v=0:a=1[a]", len(inputs))
	args = append(args, "-filter_complex", fc.String(), "-map", "[a]", out)
	return runFFmpeg(ctx, args...)
}

// MixBackgroundMusic mixes quiet looped music under the narration. The output
// length matches the narration. If musicPath is empty the narration is copied.
func MixBackgroundMusic(ctx context.Context, narrationPath, musicPath, out string) error {
	if musicPath == "" {
		return runFFmpeg(ctx, "-i", narrationPath, "-ar", "44100", out)
	}
	fc := "[1:a]volume=0.25[m];[0:a][m]amix=inputs=2:duration=first:dropout_transition=0[a]"
	return runFFmpeg(ctx,
		"-i", narrationPath,
		"-stream_loop", "-1", "-i", musicPath,
		"-filter_complex", fc,
		"-map", "[a]", "-ar", "44100", out,
	)
}

func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) }

// Overlay is a timed image overlay placed at (X,Y). If End<=0 it shows for the
// whole clip; otherwise only while Start <= t <= End.
type Overlay struct {
	PNG        string
	X, Y       int
	Start, End float64
}

// encodeArgs are the shared output encoding flags. Audio is intentionally NOT
// trimmed to the shortest stream (the bg video runs ~0.5s past the narration).
var encodeArgs = []string{
	"-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p",
	"-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart",
}

// ComposeVideo crops the background to 9:16, overlays the timed images, attaches
// the audio, and encodes — all in a single ffmpeg pass (one encode). onProgress,
// if non-nil, is called with the fraction of dur encoded so far.
func ComposeVideo(ctx context.Context, bg string, start, dur float64, cropW, cropH, cropX int,
	audio string, overlays []Overlay, fps int, out string, onProgress func(float64)) error {

	args := []string{"-ss", ftoa(start), "-t", ftoa(dur), "-i", bg, "-i", audio}
	for _, ov := range overlays {
		args = append(args, "-i", ov.PNG)
	}

	var fc bytes.Buffer
	fmt.Fprintf(&fc, "[0:v]crop=%d:%d:%d:0[v0]", cropW, cropH, cropX)
	prev := "v0"
	for i, ov := range overlays {
		next := fmt.Sprintf("v%d", i+1)
		enable := ""
		if ov.End > 0 {
			enable = fmt.Sprintf(":enable='between(t,%s,%s)'", ftoa(ov.Start), ftoa(ov.End))
		}
		fmt.Fprintf(&fc, ";[%s][%d:v]overlay=%d:%d%s[%s]", prev, 2+i, ov.X, ov.Y, enable, next)
		prev = next
	}

	args = append(args, "-filter_complex", fc.String(), "-map", "["+prev+"]", "-map", "1:a")
	args = append(args, encodeArgs...)
	args = append(args, "-r", strconv.Itoa(fps), out)
	return runFFmpegProgress(ctx, dur, onProgress, args...)
}

// ComposeSegment builds one AskReddit segment: cropped background + a centered,
// scaled, semi-transparent screenshot + the segment audio. onProgress, if
// non-nil, is called with the fraction of dur encoded so far.
func ComposeSegment(ctx context.Context, bg string, start, dur float64, cropW, cropH, cropX int,
	audio, screenshot string, scaleW, scaleH int, opacity float64, fps int, out string, onProgress func(float64)) error {

	args := []string{
		"-ss", ftoa(start), "-t", ftoa(dur), "-i", bg,
		"-i", audio,
		"-i", screenshot,
	}
	fc := fmt.Sprintf(
		"[0:v]crop=%d:%d:%d:0[b];[2:v]scale=%d:%d,format=rgba,colorchannelmixer=aa=%s[ov];[b][ov]overlay=(W-w)/2:(H-h)/2[v]",
		cropW, cropH, cropX, scaleW, scaleH, ftoa(opacity),
	)
	args = append(args, "-filter_complex", fc, "-map", "[v]", "-map", "1:a")
	args = append(args, encodeArgs...)
	args = append(args, "-r", strconv.Itoa(fps), out)
	return runFFmpegProgress(ctx, dur, onProgress, args...)
}

// ConcatSegments concatenates pre-encoded segments, optionally mixes quiet
// background music under the whole thing, and overlays a watermark. totalDur
// is the concatenated output's duration (sum of the segments'), used to scale
// onProgress, which is called (if non-nil) with the fraction encoded so far.
func ConcatSegments(ctx context.Context, segments []string, musicPath string, watermark *Overlay, totalDur float64, out string, onProgress func(float64)) error {
	args := []string{}
	for _, s := range segments {
		args = append(args, "-i", s)
	}
	musicIdx, wmIdx := -1, -1
	if musicPath != "" {
		musicIdx = len(segments)
		args = append(args, "-stream_loop", "-1", "-i", musicPath)
	}
	if watermark != nil {
		wmIdx = len(segments)
		if musicIdx >= 0 {
			wmIdx++
		}
		args = append(args, "-i", watermark.PNG)
	}

	var fc bytes.Buffer
	for i := range segments {
		fmt.Fprintf(&fc, "[%d:v][%d:a]", i, i)
	}
	fmt.Fprintf(&fc, "concat=n=%d:v=1:a=1[cv][ca]", len(segments))

	vlabel, alabel := "cv", "ca"
	if musicIdx >= 0 {
		fmt.Fprintf(&fc, ";[%d:a]volume=0.25[m];[ca][m]amix=inputs=2:duration=first:dropout_transition=0[mixa]", musicIdx)
		alabel = "mixa"
	}
	if watermark != nil {
		fmt.Fprintf(&fc, ";[cv][%d:v]overlay=%d:%d[wv]", wmIdx, watermark.X, watermark.Y)
		vlabel = "wv"
	}

	args = append(args, "-filter_complex", fc.String(), "-map", "["+vlabel+"]", "-map", "["+alabel+"]")
	args = append(args, encodeArgs...)
	args = append(args, out)
	return runFFmpeg(ctx, args...)
}
