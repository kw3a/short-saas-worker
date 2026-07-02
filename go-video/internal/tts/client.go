// Package tts is an HTTP client for the Python TTS microservice, which wraps the
// Azure Speech SDK and returns synthesized audio plus word-level timestamps.
package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Word is a single word-boundary timestamp (seconds).
type Word struct {
	Word     string  `json:"word"`
	Start    float64 `json:"start"`
	Duration float64 `json:"duration"`
}

// Result is the outcome of one synthesis call.
type Result struct {
	// AudioPath is the local path of the written WAV file.
	AudioPath string
	// Duration of the audio in seconds (sum is derived by caller as needed).
	Words []Word
}

type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

func New(baseURL, secret string) *Client {
	return &Client{
		baseURL: baseURL,
		secret:  secret,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

type synthRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
}

type synthResponse struct {
	Timestamps []Word `json:"timestamps"`
	AudioB64   string `json:"audio_b64"`
}

// Synthesize calls the TTS service and writes the returned WAV to outPath.
func (c *Client) Synthesize(ctx context.Context, text, voice, outPath string) ([]Word, error) {
	body, _ := json.Marshal(synthRequest{Text: text, Voice: voice})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/synthesize", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tts service returned %d", resp.StatusCode)
	}

	var sr synthResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("decode tts response: %w", err)
	}
	audio, err := base64.StdEncoding.DecodeString(sr.AudioB64)
	if err != nil {
		return nil, fmt.Errorf("decode audio: %w", err)
	}
	if err := os.WriteFile(outPath, audio, 0o644); err != nil {
		return nil, err
	}
	return sr.Timestamps, nil
}
