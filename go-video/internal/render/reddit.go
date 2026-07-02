package render

import (
	"fmt"
	"math/rand"
	"strings"

	"github.com/fogleman/gg"
	"golang.org/x/image/font"
)

// Port of comment_screenshot.py: light-theme Reddit comment / title images.

const ssFactor = 2 // supersampling factor (S)

func randUsername() string {
	prefixes := []string{"throwaway", "user", "anon", "just", "real", "cool", "the",
		"auto", "random", "happy", "sad", "noob", "pro"}
	return fmt.Sprintf("%s%d", prefixes[rand.Intn(len(prefixes))], 10+rand.Intn(99990))
}

func randTimeAgo() string {
	switch rand.Intn(3) {
	case 0:
		return fmt.Sprintf("%dm ago", 1+rand.Intn(59))
	case 1:
		return fmt.Sprintf("%dh ago", 1+rand.Intn(23))
	default:
		return fmt.Sprintf("%dd ago", 1+rand.Intn(7))
	}
}

func randLikesText() string {
	n := 1 + rand.Intn(5000)
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func randUpvotesText() string {
	n := 1 + rand.Intn(99999)
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// greedyWrap wraps text on spaces so each line fits maxW (handles \n paragraphs).
func greedyWrap(fc font.Face, text string, maxW float64) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		if strings.TrimSpace(para) == "" {
			lines = append(lines, "")
			continue
		}
		cur := ""
		for _, word := range strings.Fields(para) {
			test := word
			if cur != "" {
				test = cur + " " + word
			}
			if measureLine(fc, test) <= maxW {
				cur = test
			} else {
				if cur != "" {
					lines = append(lines, cur)
				}
				cur = word
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
	}
	return lines
}

// ayHeight approximates PIL's font.getbbox("Ay")[3] (cap-top to descent).
func ayHeight(fc font.Face) float64 {
	m := fc.Metrics()
	return float64(m.Ascent+m.Descent) / 64.0
}

func drawTL(dc *gg.Context, s string, x, y float64) { dc.DrawStringAnchored(s, x, y, 0, 0) }

// roundedWithBorder clips the rendered context to a rounded rect and optionally
// strokes a border, returning a new context.
func roundedWithBorder(src *gg.Context, radius, border float64, br, bg, bb, ba int) *gg.Context {
	w, h := src.Width(), src.Height()
	out := gg.NewContext(w, h)
	out.DrawRoundedRectangle(0, 0, float64(w), float64(h), radius)
	out.Clip()
	out.DrawImage(src.Image(), 0, 0)
	out.ResetClip()
	if border > 0 {
		out.SetRGBA255(br, bg, bb, ba)
		out.SetLineWidth(border)
		out.DrawRoundedRectangle(border/2, border/2, float64(w)-border, float64(h)-border, radius)
		out.Stroke()
	}
	return out
}

// GenerateRedditComment renders a light-theme comment screenshot to outPath.
func (r *Renderer) GenerateRedditComment(outPath, comment string) error {
	S := float64(ssFactor)
	width := int(756 * S)
	margin := 40 * S
	avatar := 48 * S

	userSz, metaSz, textSz, actionsSz, lineExtra := 20*S, 18*S, 20*S, 18*S, 6*S
	if len(comment) >= 700 {
		userSz, metaSz, textSz, actionsSz, lineExtra = 18*S, 16*S, 16*S, 16*S, 4*S
	}
	fUser, err := face(dejaVuSans, userSz)
	if err != nil {
		return err
	}
	fMeta, _ := face(dejaVuSans, metaSz)
	fText, _ := face(dejaVuSans, textSz)
	fActions, _ := face(dejaVuSans, actionsSz)

	lineX := margin - 25*S
	avatarX, avatarY := margin, margin
	textX := avatarX + avatar + 12*S
	maxTextW := float64(width) - (textX + margin)

	lines := greedyWrap(fText, comment, maxTextW)
	lineH := ayHeight(fText) + lineExtra
	totalTextH := lineH * float64(len(lines))
	totalH := int(margin + avatar + 20 + totalTextH + 80)

	dc := gg.NewContext(width, totalH)
	dc.SetRGB255(255, 255, 255)
	dc.Clear()

	// thread line
	dc.SetRGB255(224, 226, 227)
	dc.SetLineWidth(3 * S)
	dc.DrawLine(lineX, margin, lineX, float64(totalH)-20*S)
	dc.Stroke()

	// avatar (random fill circle)
	dc.SetRGB255(60+rand.Intn(140), 60+rand.Intn(140), 60+rand.Intn(140))
	dc.DrawCircle(avatarX+avatar/2, avatarY+avatar/2, avatar/2)
	dc.Fill()

	// username + time
	username := randUsername()
	dc.SetFontFace(fUser)
	dc.SetRGB255(28, 28, 28)
	drawTL(dc, username, textX, avatarY)
	userW := measureLine(fUser, username)
	dc.SetFontFace(fMeta)
	dc.SetRGB255(120, 124, 126)
	drawTL(dc, "• "+randTimeAgo(), textX+userW+8, avatarY)

	// comment text
	dc.SetFontFace(fText)
	dc.SetRGB255(28, 28, 28)
	textY := avatarY + avatar - 40
	for _, l := range lines {
		drawTL(dc, l, textX, textY)
		textY += lineH
	}

	// actions row
	actionsY := textY + 10*S
	dc.SetFontFace(fActions)
	likesText := "▲ " + randLikesText()
	dc.SetRGB255(28, 28, 28)
	drawTL(dc, likesText, textX, actionsY)
	x := textX + measureLine(fActions, likesText) + 24*S
	dc.SetRGB255(120, 124, 126)
	for _, action := range []string{"Reply", "Give Award", "Share", "..."} {
		drawTL(dc, action, x, actionsY)
		x += measureLine(fActions, action) + 30
	}

	return roundedWithBorder(dc, 16*S, 0, 0, 0, 0, 0).SavePNG(outPath)
}

// GenerateRedditTitle renders a light-theme post-header screenshot to outPath.
func (r *Renderer) GenerateRedditTitle(outPath, subreddit, title string) error {
	S := float64(ssFactor)
	width := int(756 * S)
	margin := 24 * S

	fMetaBold, err := face(dejaVuSansBold, 18*S)
	if err != nil {
		return err
	}
	fMeta, _ := face(dejaVuSans, 18*S)
	fTitle, _ := face(dejaVuSans, 28*S)

	maxW := float64(width) - margin*2
	lines := greedyWrap(fTitle, title, maxW)
	lineH := ayHeight(fTitle) + 6*S
	titleH := lineH * float64(max(1, len(lines)))
	if titleH < 40 {
		titleH = 40
	}
	metaH := ayHeight(fMeta)
	totalH := int(margin + metaH + 10 + titleH + 20 + 1)

	dc := gg.NewContext(width, totalH)
	dc.SetRGB255(255, 255, 255)
	dc.Clear()

	username := randUsername()
	metaText := fmt.Sprintf("%s • Posted by u/%s • %s", subreddit, username, randTimeAgo())
	dc.SetFontFace(fMeta)
	dc.SetRGB255(120, 124, 126)
	drawTL(dc, metaText, margin, margin)

	dc.SetFontFace(fTitle)
	dc.SetRGB255(28, 28, 28)
	y := margin + metaH + 10*S
	for _, l := range lines {
		drawTL(dc, l, margin, y)
		y += lineH
	}

	upvoteText := "▲ " + randUpvotesText()
	dc.SetFontFace(fMetaBold)
	uw := measureLine(fMetaBold, upvoteText)
	dc.SetRGB255(28, 28, 28)
	drawTL(dc, upvoteText, float64(width)-margin-uw, margin)

	dc.SetRGB255(240, 241, 242)
	dc.SetLineWidth(2 * S)
	dc.DrawLine(0, float64(totalH)-1, float64(width), float64(totalH)-1)
	dc.Stroke()

	return roundedWithBorder(dc, 16*S, 2*S, 0, 0, 0, 255).SavePNG(outPath)
}
