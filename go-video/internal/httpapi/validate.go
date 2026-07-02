package httpapi

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/viralshort/go-video/internal/render"
)

// Allowed values, mirrored from the Python service (main.py).
var (
	validBgVideos = set("gtav", "minecraft", "roblox", "subways", "satisfying")
	validVoices   = set(
		"en-US-BrianNeural", "en-US-AvaNeural", "en-US-AndrewNeural",
		"en-US-EmmaNeural", "en-US-JennyNeural",
		"es-BO-SofiaNeural", "es-BO-MarceloNeural",
		"es-MX-JorgeNeural", "es-MX-DaliaNeural", "es-DO-EmilioNeural",
	)
	validMusics = set("elevator", "else", "hiddenagenda", "nocturne",
		"sneakysnitch", "tiptoes", "wiener", "waltz")
)

func set(items ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, it := range items {
		m[it] = struct{}{}
	}
	return m
}

// narrationRequest / askRedditRequest are the wire DTOs.
type narrationRequest struct {
	ID        string  `json:"id"`
	Script    string  `json:"script"`
	Title     *string `json:"title"`
	BgVideo   string  `json:"bg_video"`
	Voice     string  `json:"voice"`
	Music     *string `json:"music"`
	FreeTrial *bool   `json:"free_trial"`
}

type askRedditRequest struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Comments  []string `json:"comments"`
	BgVideo   string   `json:"bg_video"`
	Voice     string   `json:"voice"`
	Music     *string  `json:"music"`
	FreeTrial *bool    `json:"free_trial"`
}

func (r narrationRequest) validate() (render.NarrationJob, error) {
	id, err := uuid.Parse(strings.TrimSpace(r.ID))
	if err != nil {
		return render.NarrationJob{}, errors.New("id must be a valid UUID")
	}
	script := strings.TrimSpace(r.Script)
	if l := utf8.RuneCountInString(script); l < 1 || l > 2000 {
		return render.NarrationJob{}, errors.New("script must be between 1 and 2000 characters")
	}
	title := ""
	if r.Title != nil {
		title = strings.TrimSpace(*r.Title)
		if utf8.RuneCountInString(title) > 100 {
			return render.NarrationJob{}, errors.New("title must be at most 100 characters")
		}
	}
	bg, music, err := validateBgMusic(r.BgVideo, r.Music)
	if err != nil {
		return render.NarrationJob{}, err
	}
	voice := strings.TrimSpace(r.Voice)
	if _, ok := validVoices[voice]; !ok {
		return render.NarrationJob{}, errors.New("voice is not valid")
	}
	return render.NarrationJob{
		ID: id, Script: script, Title: title, BgVideo: bg, Voice: voice,
		Music: music, FreeTrial: r.FreeTrial != nil && *r.FreeTrial,
	}, nil
}

func (r askRedditRequest) validate() (render.AskRedditJob, error) {
	id, err := uuid.Parse(strings.TrimSpace(r.ID))
	if err != nil {
		return render.AskRedditJob{}, errors.New("id must be a valid UUID")
	}
	title := strings.TrimSpace(r.Title)
	if l := utf8.RuneCountInString(title); l < 1 || l > 100 {
		return render.AskRedditJob{}, errors.New("title must be between 1 and 100 characters")
	}
	if len(r.Comments) < 1 || len(r.Comments) > 20 {
		return render.AskRedditJob{}, errors.New("comments count must be between 1 and 20")
	}
	cleaned := make([]string, 0, len(r.Comments))
	total := 0
	for _, c := range r.Comments {
		c2 := strings.TrimSpace(c)
		l := utf8.RuneCountInString(c2)
		if l < 1 || l > 1000 {
			return render.AskRedditJob{}, errors.New("each comment length must be between 1 and 1000 characters")
		}
		cleaned = append(cleaned, c2)
		total += l
	}
	if total >= 2000 {
		return render.AskRedditJob{}, errors.New("sum of comments length must be less than 2000 characters")
	}
	bg, music, err := validateBgMusic(r.BgVideo, r.Music)
	if err != nil {
		return render.AskRedditJob{}, err
	}
	voice := strings.TrimSpace(r.Voice)
	if _, ok := validVoices[voice]; !ok {
		return render.AskRedditJob{}, errors.New("voice is not valid")
	}
	return render.AskRedditJob{
		ID: id, Title: title, Comments: cleaned, BgVideo: bg, Voice: voice,
		Music: music, FreeTrial: r.FreeTrial != nil && *r.FreeTrial,
	}, nil
}

func validateBgMusic(bgVideo string, music *string) (string, string, error) {
	bg := strings.TrimSpace(bgVideo)
	if _, ok := validBgVideos[bg]; !ok {
		return "", "", errors.New("bg_video is not valid")
	}
	m := ""
	if music != nil && strings.TrimSpace(*music) != "" {
		m = strings.TrimSpace(*music)
		if _, ok := validMusics[m]; !ok {
			return "", "", errors.New("music is not valid")
		}
	}
	return bg, m, nil
}
