package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/viralshort/go-video/internal/config"
	"github.com/viralshort/go-video/internal/httpapi"
	"github.com/viralshort/go-video/internal/queue"
	"github.com/viralshort/go-video/internal/render"
	"github.com/viralshort/go-video/internal/storage"
	"github.com/viralshort/go-video/internal/store"
	"github.com/viralshort/go-video/internal/tts"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Worker lifetime context: render jobs keep running until shutdown.
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()

	st, err := store.New(workerCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	r2, err := storage.NewR2(workerCtx, cfg.R2AccountID, cfg.R2AccessKeyID, cfg.R2SecretKey, cfg.R2Bucket, cfg.R2Endpoint)
	if err != nil {
		return err
	}

	deps := &render.Deps{
		TTS:       tts.New(cfg.TTSBaseURL, cfg.TTSSecret),
		R2:        r2,
		Store:     st,
		Renderer:  render.NewRenderer(cfg.AssetsDir),
		AssetsDir: cfg.AssetsDir,
	}

	q := queue.New(cfg.OutputWorkers, cfg.OutputWorkers*8)
	q.Start(workerCtx)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewServer(cfg.ServerSecret, deps, q).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("go-video listening on :%s (workers=%d)", cfg.Port, cfg.OutputWorkers)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// Graceful shutdown: stop HTTP, drain in-flight renders, then exit.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)

	q.Shutdown()  // wait for in-flight renders
	stopWorkers() // cancel worker context
	log.Println("bye")
	return nil
}
