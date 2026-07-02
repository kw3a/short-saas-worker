package render

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
}

func fileNotEmpty(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
	if fi.Size() == 0 {
		t.Fatalf("expected %s to be non-empty", path)
	}
}

// renderer pointed at the repo assets dir (font lives there).
func testRenderer() *Renderer { return NewRenderer("../../assets") }

func TestRenderOverlays(t *testing.T) {
	r := testRenderer()
	dir := t.TempDir()

	word := filepath.Join(dir, "word.png")
	if _, err := r.WordSubtitle(word, "Supercalifragilistic", 1080); err != nil {
		t.Fatalf("WordSubtitle: %v", err)
	}
	fileNotEmpty(t, word)

	title := filepath.Join(dir, "title.png")
	if _, err := r.TitleCard(title, "What is something you learned late in life?", 1080, 1920); err != nil {
		t.Fatalf("TitleCard: %v", err)
	}
	fileNotEmpty(t, title)

	wm := filepath.Join(dir, "wm.png")
	if _, err := r.Watermark(wm, 1080, 1920); err != nil {
		t.Fatalf("Watermark: %v", err)
	}
	fileNotEmpty(t, wm)

	comment := filepath.Join(dir, "comment.png")
	if err := r.GenerateRedditComment(comment, "This is a sample reddit comment used for testing the renderer."); err != nil {
		t.Fatalf("GenerateRedditComment: %v", err)
	}
	fileNotEmpty(t, comment)

	rtitle := filepath.Join(dir, "rtitle.png")
	if err := r.GenerateRedditTitle(rtitle, "r/AskReddit", "What is the weirdest thing you've seen?"); err != nil {
		t.Fatalf("GenerateRedditTitle: %v", err)
	}
	fileNotEmpty(t, rtitle)
}

func TestComposePipeline(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()
	dir := t.TempDir()
	p := func(n string) string { return filepath.Join(dir, n) }

	// Synthetic landscape background + 1s silent narration.
	bg := p("bg.mp4")
	if err := runFFmpeg(ctx, "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30:duration=4",
		"-pix_fmt", "yuv420p", bg); err != nil {
		t.Fatalf("make bg: %v", err)
	}
	narration := p("narration.wav")
	if err := runFFmpeg(ctx, "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "1.0", narration); err != nil {
		t.Fatalf("make audio: %v", err)
	}

	bgInfo, err := Probe(ctx, bg)
	if err != nil {
		t.Fatalf("probe bg: %v", err)
	}
	if bgInfo.Width != 1280 || bgInfo.Height != 720 {
		t.Fatalf("unexpected bg size: %dx%d", bgInfo.Width, bgInfo.Height)
	}
	cw, ch, cx := cropDims(bgInfo.Width, bgInfo.Height)
	if cw != 404 || ch != 720 { // 720*9/16 = 405 -> even 404
		t.Fatalf("unexpected crop: %dx%d (x=%d)", cw, ch, cx)
	}

	// One subtitle overlay.
	r := testRenderer()
	word := p("w.png")
	sz, err := r.WordSubtitle(word, "Hello", cw)
	if err != nil {
		t.Fatalf("WordSubtitle: %v", err)
	}
	overlays := []Overlay{{PNG: word, X: (cw - sz.W) / 2, Y: (ch - sz.H) / 2, Start: 0, End: 1.0}}

	start, vidLen := buildWindow(bgInfo.Duration, 1.0)
	out := p("out.mp4")
	var fracs []float64
	onProgress := func(f float64) { fracs = append(fracs, f) }
	if err := ComposeVideo(ctx, bg, start, vidLen, cw, ch, cx, narration, overlays, outputFPS, out, onProgress); err != nil {
		t.Fatalf("ComposeVideo: %v", err)
	}
	fileNotEmpty(t, out)
	if len(fracs) == 0 {
		t.Fatal("onProgress was never called")
	}
	for i := 1; i < len(fracs); i++ {
		if fracs[i] < fracs[i-1] {
			t.Fatalf("progress not monotonic: %v", fracs)
		}
	}
	// The last reported fraction won't be exactly 1.0: frame-rate quantization
	// means ffmpeg's actual encoded duration falls slightly short of the
	// requested -t (callers should treat "the compose call returned" as the
	// authoritative completion signal, not waiting for frac==1).
	if last := fracs[len(fracs)-1]; last < 0.9 || last > 1.0 {
		t.Fatalf("expected final progress near 1.0, got %v", last)
	}

	outInfo, err := Probe(ctx, out)
	if err != nil {
		t.Fatalf("probe out: %v", err)
	}
	if outInfo.Width != cw || outInfo.Height != ch {
		t.Fatalf("output not 9:16 cropped: %dx%d", outInfo.Width, outInfo.Height)
	}
	if outInfo.Duration < 1.4 || outInfo.Duration > 1.7 { // ~1.0 + 0.5 tail
		t.Fatalf("unexpected output duration: %.3f", outInfo.Duration)
	}

	// Thumbnail.
	thumb := p("thumb.jpg")
	if err := Thumbnail(ctx, out, thumb); err != nil {
		t.Fatalf("Thumbnail: %v", err)
	}
	fileNotEmpty(t, thumb)
}

func TestConcatSegments(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()
	dir := t.TempDir()
	p := func(n string) string { return filepath.Join(dir, n) }

	bg := p("bg.mp4")
	if err := runFFmpeg(ctx, "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30:duration=4",
		"-pix_fmt", "yuv420p", bg); err != nil {
		t.Fatalf("make bg: %v", err)
	}
	cw, ch, cx := cropDims(1280, 720)
	r := testRenderer()

	var segs []string
	for i := range 2 {
		audio := p("a.wav")
		if err := runFFmpeg(ctx, "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "0.8", audio); err != nil {
			t.Fatalf("audio: %v", err)
		}
		shot := p("s.png")
		if err := r.GenerateRedditComment(shot, "segment comment text"); err != nil {
			t.Fatalf("shot: %v", err)
		}
		si, _ := Probe(ctx, shot)
		sw, sh := fitScale(si.Width, si.Height, int(float64(cw)*0.88), int(float64(ch)*0.78))
		seg := p("seg" + string(rune('0'+i)) + ".mp4")
		if err := ComposeSegment(ctx, bg, 0, 1.3, cw, ch, cx, audio, shot, sw, sh, 0.9, outputFPS, seg, nil); err != nil {
			t.Fatalf("ComposeSegment: %v", err)
		}
		segs = append(segs, seg)
	}

	out := p("final.mp4")
	if err := ConcatSegments(ctx, segs, "", nil, 2.6, out, nil); err != nil {
		t.Fatalf("ConcatSegments: %v", err)
	}
	fileNotEmpty(t, out)
	info, err := Probe(ctx, out)
	if err != nil {
		t.Fatalf("probe final: %v", err)
	}
	if info.Duration < 2.0 { // two ~1.3s segments
		t.Fatalf("unexpected concat duration: %.3f", info.Duration)
	}
}
