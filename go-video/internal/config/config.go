package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration, sourced from environment variables.
type Config struct {
	Port          string
	ServerSecret  string // shared bearer secret for incoming requests
	DatabaseURL   string
	AssetsDir     string // root dir containing backgrounds/ and musics/
	OutputWorkers int    // number of concurrent render workers

	// Cloudflare R2 (S3-compatible)
	R2AccountID   string
	R2AccessKeyID string
	R2SecretKey   string
	R2Bucket      string
	R2Endpoint    string // overrides the derived Cloudflare endpoint, e.g. http://localhost:9000 for local MinIO

	// TTS microservice (Python)
	TTSBaseURL string
	TTSSecret  string
}

// Load reads configuration from the environment. In non-production it also loads
// a local .env file if present.
func Load() (*Config, error) {
	if os.Getenv("ENVIRONMENT") != "production" {
		_ = godotenv.Load()
	}

	c := &Config{
		Port:          getenv("PORT", "3000"),
		ServerSecret:  os.Getenv("VIDEO_SERVER_SECRET"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		AssetsDir:     getenv("ASSETS_DIR", "."),
		OutputWorkers: getenvInt("RENDER_WORKERS", 2),

		R2AccountID:   os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKeyID: os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretKey:   os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:      getenv("R2_BUCKET_NAME", "shorts"),
		R2Endpoint:    os.Getenv("R2_ENDPOINT"),

		TTSBaseURL: getenv("TTS_BASE_URL", "http://localhost:3001"),
		TTSSecret:  getenv("TTS_SECRET", os.Getenv("VIDEO_SERVER_SECRET")),
	}

	var missing []string
	for k, v := range map[string]string{
		"VIDEO_SERVER_SECRET":  c.ServerSecret,
		"DATABASE_URL":         c.DatabaseURL,
		"R2_ACCOUNT_ID":        c.R2AccountID,
		"R2_ACCESS_KEY_ID":     c.R2AccessKeyID,
		"R2_SECRET_ACCESS_KEY": c.R2SecretKey,
	} {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env: %v", missing)
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
