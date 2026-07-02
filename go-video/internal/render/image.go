package render

import (
	"math"
	"path/filepath"
	"strings"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
)

// Renderer rasterizes subtitle/title/watermark overlays with gg, mirroring the
// look produced by the original Pillow code.
type Renderer struct {
	assetsDir string
}

func NewRenderer(assetsDir string) *Renderer { return &Renderer{assetsDir: assetsDir} }

func (r *Renderer) montserrat() string { return filepath.Join(r.assetsDir, montserratRel) }

// imgSize is the pixel size of a rendered overlay.
type imgSize struct{ W, H int }

// measureLine returns the rendered width of a single line for a face.
func measureLine(face font.Face, s string) float64 {
	dc := gg.NewContext(1, 1)
	dc.SetFontFace(face)
	w, _ := dc.MeasureString(s)
	return w
}

func lineHeight(face font.Face) float64 {
	return float64(face.Metrics().Height) / 64.0
}

// drawOutlinedString draws text with a stroke (outline) by stamping the stroke
// color around the glyph, then the fill color on top. Anchored at (ax, ay).
func drawOutlinedString(dc *gg.Context, s string, x, y, ax, ay float64, strokeW int,
	fillR, fillG, fillB, fillA int, strokeR, strokeG, strokeB, strokeA int) {
	if strokeW > 0 {
		dc.SetRGBA255(strokeR, strokeG, strokeB, strokeA)
		sw := float64(strokeW)
		for dy := -strokeW; dy <= strokeW; dy++ {
			for dx := -strokeW; dx <= strokeW; dx++ {
				if dx == 0 && dy == 0 {
					continue
				}
				if float64(dx*dx+dy*dy) > sw*sw {
					continue
				}
				dc.DrawStringAnchored(s, x+float64(dx), y+float64(dy), ax, ay)
			}
		}
	}
	dc.SetRGBA255(fillR, fillG, fillB, fillA)
	dc.DrawStringAnchored(s, x, y, ax, ay)
}

// hyphenWrap splits a single long word so each line fits within maxPx, adding a
// trailing hyphen on continued lines (mirrors wrap_long_word_to_width).
func hyphenWrap(face font.Face, word string, maxPx float64) []string {
	if measureLine(face, word) <= maxPx {
		return []string{word}
	}
	runes := []rune(word)
	var lines []string
	start := 0
	for start < len(runes) {
		end := start + 1
		lastFit := start
		for end <= len(runes) {
			cand := string(runes[start:end])
			if end < len(runes) {
				cand += "-"
			}
			if measureLine(face, cand) <= maxPx {
				lastFit = end
				end++
			} else {
				break
			}
		}
		if lastFit == start {
			lastFit = min(start+1, len(runes))
		}
		line := string(runes[start:lastFit])
		if lastFit < len(runes) {
			line += "-"
		}
		lines = append(lines, line)
		start = lastFit
	}
	return lines
}

// WordSubtitle renders one script word: uppercase white text with a black stroke
// on a rounded orange card. Returns the PNG path and its size.
func (r *Renderer) WordSubtitle(outPath, word string, frameW int) (imgSize, error) {
	const fontSize = 56.0
	const strokeW = 4
	const padX, padY = 10.0, 6.0
	const radius = 14.0

	fc, err := face(r.montserrat(), fontSize)
	if err != nil {
		return imgSize{}, err
	}
	text := strings.ToUpper(word)
	maxTextW := float64(frameW) * 0.9
	lines := hyphenWrap(fc, text, maxTextW)

	lh := lineHeight(fc)
	maxW := 0.0
	for _, l := range lines {
		if w := measureLine(fc, l); w > maxW {
			maxW = w
		}
	}
	textH := lh * float64(len(lines))
	bgW := int(math.Ceil(maxW + 2*padX))
	bgH := int(math.Ceil(textH + 2*padY))

	dc := gg.NewContext(bgW, bgH)
	dc.SetFontFace(fc)
	// orange card
	dc.SetRGBA255(255, 90, 60, 230)
	dc.DrawRoundedRectangle(0, 0, float64(bgW), float64(bgH), radius)
	dc.Fill()
	// centered, outlined text
	cx := float64(bgW) / 2
	for i, l := range lines {
		y := padY + lh*float64(i) + lh/2
		drawOutlinedString(dc, l, cx, y, 0.5, 0.5, strokeW,
			255, 255, 255, 255, 0, 0, 0, 255)
	}
	if err := dc.SavePNG(outPath); err != nil {
		return imgSize{}, err
	}
	return imgSize{bgW, bgH}, nil
}

// TitleCard renders the title: two translucent rounded cards (orange, white),
// rotated slightly opposite ways, with the black caption centered on top.
func (r *Renderer) TitleCard(outPath, title string, frameW, frameH int) (imgSize, error) {
	const padX, padY = 20.0, 16.0
	const radius = 20.0
	maxW := float64(frameW) * 0.80
	maxH := float64(frameH) * 0.55

	text := strings.ToUpper(title)
	fs := 56.0
	var fc font.Face
	var lines []string
	var lh, maxLineW, textH float64
	for {
		var err error
		fc, err = face(r.montserrat(), fs)
		if err != nil {
			return imgSize{}, err
		}
		dc := gg.NewContext(1, 1)
		dc.SetFontFace(fc)
		lines = dc.WordWrap(text, maxW)
		if len(lines) == 0 {
			lines = []string{text}
		}
		lh = lineHeight(fc)
		maxLineW = 0
		for _, l := range lines {
			if w := measureLine(fc, l); w > maxLineW {
				maxLineW = w
			}
		}
		textH = lh * float64(len(lines))
		if textH+2*padY <= maxH || fs <= 24 {
			break
		}
		fs = math.Max(24, fs*0.92)
	}

	cardW := maxLineW + 2*padX
	cardH := textH + 2*padY
	// Canvas large enough that the rotated cards aren't clipped.
	margin := 0.18 * math.Max(cardW, cardH)
	cw := int(math.Ceil(cardW + 2*margin))
	ch := int(math.Ceil(cardH + 2*margin))
	cx, cy := float64(cw)/2, float64(ch)/2

	dc := gg.NewContext(cw, ch)
	dc.SetFontFace(fc)

	drawCard := func(angleDeg float64, rr, gg2, bb, aa int) {
		dc.Push()
		dc.RotateAbout(gg.Radians(angleDeg), cx, cy)
		dc.SetRGBA255(rr, gg2, bb, aa)
		dc.DrawRoundedRectangle(cx-cardW/2, cy-cardH/2, cardW, cardH, radius)
		dc.Fill()
		dc.Pop()
	}
	drawCard(-6, 255, 165, 0, 192)  // back: translucent orange
	drawCard(2, 255, 255, 255, 192) // front: translucent white

	// caption: black, ~75% opacity, centered, no rotation
	dc.SetRGBA255(0, 0, 0, 191)
	startY := cy - textH/2 + lh/2
	for i, l := range lines {
		dc.DrawStringAnchored(l, cx, startY+lh*float64(i), 0.5, 0.5)
	}

	if err := dc.SavePNG(outPath); err != nil {
		return imgSize{}, err
	}
	return imgSize{cw, ch}, nil
}

// Watermark renders the "viralshort.app" banner (play triangle + text).
func (r *Renderer) Watermark(outPath string, videoW, videoH int) (imgSize, error) {
	bannerH := max(40, int(float64(videoH)*0.07))
	paddingX := max(12, int(float64(bannerH)*0.25))
	gap := max(8, int(float64(bannerH)*0.2))
	alpha := int(0.6 * 255)

	fontSize := math.Max(16, float64(bannerH)*0.45)
	fc, err := face(dejaVuSans, fontSize)
	if err != nil {
		// best-effort: fall back to Montserrat if DejaVu is missing
		if fc, err = face(r.montserrat(), fontSize); err != nil {
			return imgSize{}, err
		}
	}
	text := "viralshort.app"
	tw := measureLine(fc, text)

	triH := int(float64(bannerH) * 0.5)
	triW := int(float64(triH) * 0.6)
	contentW := paddingX + triW + gap + int(tw) + paddingX
	maxW := int(float64(videoW) * 0.9)
	bannerW := min(max(240, contentW), maxW)

	dc := gg.NewContext(bannerW, bannerH)
	dc.SetFontFace(fc)
	dc.SetRGBA255(0, 0, 0, alpha)
	dc.DrawRectangle(0, 0, float64(bannerW), float64(bannerH))
	dc.Fill()

	cy := float64(bannerH) / 2
	x0 := float64(paddingX)
	lw := math.Max(2, float64(bannerH)*0.08)
	dc.SetRGBA255(255, 255, 255, 255)
	dc.SetLineWidth(lw)
	dc.MoveTo(x0, cy-float64(triH)/2)
	dc.LineTo(x0, cy+float64(triH)/2)
	dc.LineTo(x0+float64(triW), cy)
	dc.ClosePath()
	dc.Stroke()

	dc.DrawStringAnchored(text, x0+float64(triW)+float64(gap), cy, 0, 0.5)

	if err := dc.SavePNG(outPath); err != nil {
		return imgSize{}, err
	}
	return imgSize{bannerW, bannerH}, nil
}
