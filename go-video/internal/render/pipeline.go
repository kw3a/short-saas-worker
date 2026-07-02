package render

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/viralshort/go-video/internal/tts"
)

const outputFPS = 30
const timeDelta = 0.5 // extra background tail after narration, matching Python

// Uploader stores rendered artifacts (R2 in prod, a fake in tests).
type Uploader interface {
	UploadVideo(ctx context.Context, localPath, id string) error
	UploadThumbnail(ctx context.Context, localPath, id string) error
}

// StatusStore persists a video's render status and progress.
type StatusStore interface {
	SetStatus(ctx context.Context, id uuid.UUID, status string)
	SetProgress(ctx context.Context, id uuid.UUID, pct int)
}

// Synthesizer produces speech audio + word timestamps (the TTS HTTP client in
// prod, a fake server in tests).
type Synthesizer interface {
	Synthesize(ctx context.Context, text, voice, outPath string) ([]tts.Word, error)
}

// Deps bundles everything the render pipelines need. The external boundaries are
// interfaces so tests can simulate them while exercising the real pipeline.
type Deps struct {
	TTS       Synthesizer
	R2        Uploader
	Store     StatusStore
	Renderer  *Renderer
	AssetsDir string
}

// NarrationJob is a validated narration render request.
type NarrationJob struct {
	ID        uuid.UUID
	Script    string
	Title     string
	BgVideo   string
	Voice     string
	Music     string
	FreeTrial bool
}

// AskRedditJob is a validated AskReddit render request.
type AskRedditJob struct {
	ID        uuid.UUID
	Title     string
	Comments  []string
	BgVideo   string
	Voice     string
	Music     string
	FreeTrial bool
}

func (d *Deps) bgPath(name string) string {
	return filepath.Join(d.AssetsDir, "backgrounds", name+".mp4")
}

// musicPath returns the music file path, or "" if unset/missing.
func (d *Deps) musicPath(name string) string {
	if name == "" {
		return ""
	}
	p := filepath.Join(d.AssetsDir, "musics", name+".mp3")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// cropDims computes a centered 9:16 crop with even dimensions.
func cropDims(w, h int) (cw, ch, cx int) {
	cw = h * 9 / 16
	cw -= cw % 2
	if cw > w {
		cw = w - (w % 2)
	}
	ch = h - (h % 2)
	cx = (w - cw) / 2
	cx -= cx % 2
	return
}

// buildWindow picks a random window of length audioDur+timeDelta within vidDur.
func buildWindow(vidDur, audioDur float64) (start, vidLen float64) {
	vidLen = audioDur + timeDelta
	if vidLen > vidDur {
		vidLen = vidDur
	}
	if maxStart := vidDur - vidLen; maxStart > 0 {
		start = rand.Float64() * maxStart
	}
	return
}

// progressReporter builds an ffmpeg onProgress callback for one encode pass:
// it maps that pass's local [0,1] fraction into an overall [lo,hi] percent and
// forwards only distinct percentages, to keep DB writes down to ~100 total.
func progressReporter(report func(pct int), lo, hi int) func(float64) {
	last := -1
	return func(frac float64) {
		pct := lo + int(frac*float64(hi-lo))
		if pct == last {
			return
		}
		last = pct
		report(pct)
	}
}

// RunNarration renders a narration video end-to-end and updates DB status.
func (d *Deps) RunNarration(ctx context.Context, job NarrationJob) {
	id := job.ID.String()
	log.Printf("[%s] narration started (bg=%s voice=%s music=%s)", id, job.BgVideo, job.Voice, job.Music)
	d.Store.SetStatus(ctx, job.ID, "rendering")
	d.Store.SetProgress(ctx, job.ID, 0)

	if err := d.runNarration(ctx, job); err != nil {
		log.Printf("[%s] narration failed: %v", id, err)
		d.Store.SetStatus(ctx, job.ID, "failed")
		return
	}
	d.Store.SetProgress(ctx, job.ID, 100)
	d.Store.SetStatus(ctx, job.ID, "completed")
	log.Printf("[%s] narration completed", id)
}

func (d *Deps) runNarration(ctx context.Context, job NarrationJob) error {
	id := job.ID.String()
	setPct := func(pct int) { d.Store.SetProgress(ctx, job.ID, pct) }

	bg := d.bgPath(job.BgVideo)
	if _, err := os.Stat(bg); err != nil {
		return fmt.Errorf("background video not found: %s", job.BgVideo)
	}

	tmp, err := os.MkdirTemp("", "vid_"+id+"_")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	p := func(name string) string { return filepath.Join(tmp, name) }

	// 1) TTS for title (optional) + script.
	var titleDur float64
	var audioParts []string
	if job.Title != "" {
		titlePath := p("title.wav")
		if _, err := d.TTS.Synthesize(ctx, job.Title, job.Voice, titlePath); err != nil {
			return fmt.Errorf("tts title: %w", err)
		}
		info, err := Probe(ctx, titlePath)
		if err != nil {
			return err
		}
		titleDur = info.Duration
		audioParts = append(audioParts, titlePath)
	}
	scriptPath := p("script.wav")
	words, err := d.TTS.Synthesize(ctx, job.Script, job.Voice, scriptPath)
	if err != nil {
		return fmt.Errorf("tts script: %w", err)
	}
	audioParts = append(audioParts, scriptPath)
	setPct(15)

	// 2) Concatenate narration audio, then mix optional background music.
	combined := p("combined.wav")
	if err := ConcatAudio(ctx, audioParts, combined); err != nil {
		return fmt.Errorf("concat audio: %w", err)
	}
	combinedInfo, err := Probe(ctx, combined)
	if err != nil {
		return err
	}
	narration := p("narration.wav")
	if err := MixBackgroundMusic(ctx, combined, d.musicPath(job.Music), narration); err != nil {
		return fmt.Errorf("mix music: %w", err)
	}
	setPct(20)

	// 3) Probe background and pick the window.
	bgInfo, err := Probe(ctx, bg)
	if err != nil {
		return err
	}
	cw, ch, cx := cropDims(bgInfo.Width, bgInfo.Height)
	start, vidLen := buildWindow(bgInfo.Duration, combinedInfo.Duration)

	// 4) Render subtitle overlays (title card first, then word-by-word).
	var overlays []Overlay
	if job.Title != "" {
		png := p("title.png")
		sz, err := d.Renderer.TitleCard(png, job.Title, cw, ch)
		if err != nil {
			return fmt.Errorf("title card: %w", err)
		}
		overlays = append(overlays, Overlay{
			PNG: png, X: (cw - sz.W) / 2, Y: (ch - sz.H) / 2,
			Start: 0, End: titleDur,
		})
	}
	for i, w := range words {
		if w.Word == "" {
			continue
		}
		png := p(fmt.Sprintf("w%d.png", i))
		sz, err := d.Renderer.WordSubtitle(png, w.Word, cw)
		if err != nil {
			return fmt.Errorf("word subtitle: %w", err)
		}
		st := w.Start + titleDur
		overlays = append(overlays, Overlay{
			PNG: png, X: (cw - sz.W) / 2, Y: (ch - sz.H) / 2,
			Start: st, End: st + w.Duration,
		})
	}

	// 5) Watermark (free trial).
	if job.FreeTrial {
		png := p("wm.png")
		if sz, err := d.Renderer.Watermark(png, cw, ch); err == nil {
			overlays = append(overlays, Overlay{
				PNG: png, X: (cw - sz.W) / 2, Y: ch - sz.H - 24,
			})
		} else {
			log.Printf("[%s] watermark render failed: %v (continuing)", id, err)
		}
	}

	// 6) Single-pass compose + encode, then thumbnail + upload.
	setPct(30)
	out := p(id + ".mp4")
	onEncode := progressReporter(setPct, 30, 92)
	if err := ComposeVideo(ctx, bg, start, vidLen, cw, ch, cx, narration, overlays, outputFPS, out, onEncode); err != nil {
		return fmt.Errorf("compose: %w", err)
	}
	setPct(92)
	return d.finishAndUpload(ctx, job.ID, out, p)
}

// RunAskReddit renders an AskReddit video end-to-end and updates DB status.
func (d *Deps) RunAskReddit(ctx context.Context, job AskRedditJob) {
	id := job.ID.String()
	log.Printf("[%s] askreddit started (bg=%s voice=%s music=%s)", id, job.BgVideo, job.Voice, job.Music)
	d.Store.SetStatus(ctx, job.ID, "rendering")
	d.Store.SetProgress(ctx, job.ID, 0)

	if err := d.runAskReddit(ctx, job); err != nil {
		log.Printf("[%s] askreddit failed: %v", id, err)
		d.Store.SetStatus(ctx, job.ID, "failed")
		return
	}
	d.Store.SetProgress(ctx, job.ID, 100)
	d.Store.SetStatus(ctx, job.ID, "completed")
	log.Printf("[%s] askreddit completed", id)
}

func (d *Deps) runAskReddit(ctx context.Context, job AskRedditJob) error {
	id := job.ID.String()
	setPct := func(pct int) { d.Store.SetProgress(ctx, job.ID, pct) }

	bg := d.bgPath(job.BgVideo)
	if _, err := os.Stat(bg); err != nil {
		return fmt.Errorf("background video not found: %s", job.BgVideo)
	}
	bgInfo, err := Probe(ctx, bg)
	if err != nil {
		return err
	}
	cw, ch, cx := cropDims(bgInfo.Width, bgInfo.Height)
	setPct(5)

	tmp, err := os.MkdirTemp("", "ask_"+id+"_")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	p := func(name string) string { return filepath.Join(tmp, name) }

	type segReq struct {
		text    string
		isTitle bool
	}
	var reqs []segReq
	if job.Title != "" {
		reqs = append(reqs, segReq{job.Title, true})
	}
	for _, c := range job.Comments {
		if c != "" {
			reqs = append(reqs, segReq{c, false})
		}
	}
	if len(reqs) == 0 {
		return fmt.Errorf("no segments to build")
	}

	// Segment encodes share the 5-80% range evenly (by count, not duration —
	// durations aren't known until each segment's TTS call returns).
	const segRangeLo, segRangeHi = 5, 80
	var segments []string
	var totalDur float64
	for i, r := range reqs {
		audio := p(fmt.Sprintf("seg_%d.wav", i))
		if _, err := d.TTS.Synthesize(ctx, r.text, job.Voice, audio); err != nil {
			return fmt.Errorf("tts segment %d: %w", i, err)
		}
		ai, err := Probe(ctx, audio)
		if err != nil {
			return err
		}

		shot := p(fmt.Sprintf("shot_%d.png", i))
		if r.isTitle {
			err = d.Renderer.GenerateRedditTitle(shot, "r/AskReddit", r.text)
		} else {
			err = d.Renderer.GenerateRedditComment(shot, r.text)
		}
		if err != nil {
			return fmt.Errorf("screenshot %d: %w", i, err)
		}
		si, err := Probe(ctx, shot)
		if err != nil {
			return err
		}
		sw, sh := fitScale(si.Width, si.Height, int(float64(cw)*0.88), int(float64(ch)*0.78))

		start, vidLen := buildWindow(bgInfo.Duration, ai.Duration)
		totalDur += vidLen
		lo := segRangeLo + i*(segRangeHi-segRangeLo)/len(reqs)
		hi := segRangeLo + (i+1)*(segRangeHi-segRangeLo)/len(reqs)
		seg := p(fmt.Sprintf("seg_%d.mp4", i))
		if err := ComposeSegment(ctx, bg, start, vidLen, cw, ch, cx, audio, shot, sw, sh, 0.90, outputFPS, seg, progressReporter(setPct, lo, hi)); err != nil {
			return fmt.Errorf("compose segment %d: %w", i, err)
		}
		segments = append(segments, seg)
	}
	setPct(segRangeHi)

	var wm *Overlay
	if job.FreeTrial {
		png := p("wm.png")
		if sz, err := d.Renderer.Watermark(png, cw, ch); err == nil {
			wm = &Overlay{PNG: png, X: (cw - sz.W) / 2, Y: ch - sz.H - 24}
		}
	}

	out := p(id + ".mp4")
	onConcat := progressReporter(setPct, segRangeHi, 96)
	if err := ConcatSegments(ctx, segments, d.musicPath(job.Music), wm, totalDur, out, onConcat); err != nil {
		return fmt.Errorf("concat segments: %w", err)
	}
	setPct(96)
	return d.finishAndUpload(ctx, job.ID, out, p)
}

// finishAndUpload makes a thumbnail and uploads both artifacts to R2.
func (d *Deps) finishAndUpload(ctx context.Context, id uuid.UUID, videoPath string, p func(string) string) error {
	key := id.String()
	if err := d.R2.UploadVideo(ctx, videoPath, key); err != nil {
		return fmt.Errorf("upload video: %w", err)
	}
	thumb := p(key + ".jpg")
	if err := Thumbnail(ctx, videoPath, thumb); err != nil {
		log.Printf("[%s] thumbnail failed: %v (continuing)", key, err)
		return nil
	}
	if err := d.R2.UploadThumbnail(ctx, thumb, key); err != nil {
		log.Printf("[%s] thumbnail upload failed: %v (continuing)", key, err)
	}
	return nil
}

// fitScale scales (w,h) to fit within (maxW,maxH), never upscaling, even dims.
func fitScale(w, h, maxW, maxH int) (int, int) {
	scale := 1.0
	if sw := float64(maxW) / float64(w); sw < scale {
		scale = sw
	}
	if sh := float64(maxH) / float64(h); sh < scale {
		scale = sh
	}
	nw := int(float64(w) * scale)
	nh := int(float64(h) * scale)
	nw -= nw % 2
	nh -= nh % 2
	if nw < 2 {
		nw = 2
	}
	if nh < 2 {
		nh = 2
	}
	return nw, nh
}
