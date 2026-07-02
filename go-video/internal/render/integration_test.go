package render

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/viralshort/go-video/internal/store"
	"github.com/viralshort/go-video/internal/tts"
)

const schemaSQL = `CREATE TABLE IF NOT EXISTS video (
    id uuid PRIMARY KEY,
    user_id text NOT NULL,
    type text NOT NULL,
    credit_cost integer,
    status text NOT NULL DEFAULT 'queued',
    progress integer NOT NULL DEFAULT 0,
    created_at timestamp NOT NULL DEFAULT now(),
    updated_at timestamp NOT NULL DEFAULT now()
);`

// fakeUploader records uploads instead of hitting R2.
type fakeUploader struct {
	mu     sync.Mutex
	videos []string
	thumbs []string
}

func (f *fakeUploader) UploadVideo(_ context.Context, _, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.videos = append(f.videos, id)
	return nil
}
func (f *fakeUploader) UploadThumbnail(_ context.Context, _, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.thumbs = append(f.thumbs, id)
	return nil
}
func (f *fakeUploader) videoCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.videos)
}

// fakeTTS serves /synthesize with a fixed WAV (base64) and canned word timings,
// simulating the Python Azure service.
func fakeTTS(t *testing.T, wavB64 string) *httptest.Server {
	words := []tts.Word{
		{Word: "this", Start: 0.0, Duration: 0.30},
		{Word: "is", Start: 0.30, Duration: 0.20},
		{Word: "a", Start: 0.50, Duration: 0.15},
		{Word: "test", Start: 0.65, Duration: 0.35},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer ttssecret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"timestamps": words,
			"audio_b64":  wavB64,
		})
	}))
}

// setupAssets builds a temp ASSETS_DIR with a background video and the font.
func setupAssets(t *testing.T, ctx context.Context) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backgrounds"), 0o755); err != nil {
		t.Fatal(err)
	}
	bg := filepath.Join(dir, "backgrounds", "gtav.mp4")
	if err := runFFmpeg(ctx, "-f", "lavfi", "-i", "testsrc=size=1280x720:rate=30:duration=10",
		"-pix_fmt", "yuv420p", bg); err != nil {
		t.Fatalf("make bg: %v", err)
	}
	// font: copy from repo assets so Renderer(assetsDir) finds it
	src, err := os.ReadFile(filepath.Join("../../assets", montserratRel))
	if err != nil {
		t.Fatalf("read font: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, montserratRel), src, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func makeSilentWavB64(t *testing.T, ctx context.Context) string {
	t.Helper()
	dir := t.TempDir()
	wav := filepath.Join(dir, "s.wav")
	if err := runFFmpeg(ctx, "-f", "lavfi", "-i", "anullsrc=r=44100:cl=stereo", "-t", "1.2", wav); err != nil {
		t.Fatalf("make wav: %v", err)
	}
	b, err := os.ReadFile(wav)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// startLocalDB spins a real Postgres in a container and applies the schema.
func startLocalDB(t *testing.T, ctx context.Context) (*store.Store, *pgxpool.Pool) {
	t.Helper()
	requireFFmpeg(t)

	container, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("video"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		t.Skipf("skipping integration test (no usable Docker): %v", err)
	}
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, schemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}

	st, err := store.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	return st, pool
}

func insertVideo(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID, typ string) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO video (id, user_id, type, status) VALUES ($1, 'u', $2, 'queued')`, id, typ)
	if err != nil {
		t.Fatalf("insert video: %v", err)
	}
}

func statusOf(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()
	var s string
	if err := pool.QueryRow(ctx, `SELECT status FROM video WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatalf("query status: %v", err)
	}
	return s
}

func progressOf(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) int {
	t.Helper()
	var p int
	if err := pool.QueryRow(ctx, `SELECT progress FROM video WHERE id=$1`, id).Scan(&p); err != nil {
		t.Fatalf("query progress: %v", err)
	}
	return p
}

func newDeps(t *testing.T, ctx context.Context, st *store.Store, up Uploader, ttsURL, assets string) *Deps {
	return &Deps{
		TTS:       tts.New(ttsURL, "ttssecret"),
		R2:        up,
		Store:     st,
		Renderer:  NewRenderer(assets),
		AssetsDir: assets,
	}
}

func TestIntegrationNarration(t *testing.T) {
	ctx := context.Background()
	st, pool := startLocalDB(t, ctx)
	assets := setupAssets(t, ctx)
	ts := fakeTTS(t, makeSilentWavB64(t, ctx))
	defer ts.Close()
	up := &fakeUploader{}
	deps := newDeps(t, ctx, st, up, ts.URL, assets)

	id := uuid.New()
	insertVideo(t, ctx, pool, id, "narration")

	deps.RunNarration(ctx, NarrationJob{
		ID: id, Title: "Hello there", Script: "this is a test", BgVideo: "gtav",
		Voice: "en-US-BrianNeural", FreeTrial: true,
	})

	if got := statusOf(t, ctx, pool, id); got != "completed" {
		t.Fatalf("status = %q, want completed", got)
	}
	if got := progressOf(t, ctx, pool, id); got != 100 {
		t.Fatalf("progress = %d, want 100", got)
	}
	if up.videoCount() != 1 {
		t.Fatalf("expected 1 uploaded video, got %d", up.videoCount())
	}
}

func TestIntegrationAskReddit(t *testing.T) {
	ctx := context.Background()
	st, pool := startLocalDB(t, ctx)
	assets := setupAssets(t, ctx)
	ts := fakeTTS(t, makeSilentWavB64(t, ctx))
	defer ts.Close()
	up := &fakeUploader{}
	deps := newDeps(t, ctx, st, up, ts.URL, assets)

	id := uuid.New()
	insertVideo(t, ctx, pool, id, "askreddit")

	deps.RunAskReddit(ctx, AskRedditJob{
		ID: id, Title: "What is the weirdest thing you have seen?",
		Comments: []string{"first comment", "second comment"},
		BgVideo:  "gtav", Voice: "en-US-BrianNeural",
	})

	if got := statusOf(t, ctx, pool, id); got != "completed" {
		t.Fatalf("status = %q, want completed", got)
	}
	if got := progressOf(t, ctx, pool, id); got != 100 {
		t.Fatalf("progress = %d, want 100", got)
	}
	if up.videoCount() != 1 {
		t.Fatalf("expected 1 uploaded video, got %d", up.videoCount())
	}
}

func TestIntegrationFailureMarksFailed(t *testing.T) {
	ctx := context.Background()
	st, pool := startLocalDB(t, ctx)
	assets := setupAssets(t, ctx)
	ts := fakeTTS(t, makeSilentWavB64(t, ctx))
	defer ts.Close()
	deps := newDeps(t, ctx, st, &fakeUploader{}, ts.URL, assets)

	id := uuid.New()
	insertVideo(t, ctx, pool, id, "narration")

	// Unknown background -> pipeline fails -> status must be "failed".
	deps.RunNarration(ctx, NarrationJob{
		ID: id, Script: "this is a test", BgVideo: "does-not-exist",
		Voice: "en-US-BrianNeural",
	})

	if got := statusOf(t, ctx, pool, id); got != "failed" {
		t.Fatalf("status = %q, want failed", got)
	}
}
