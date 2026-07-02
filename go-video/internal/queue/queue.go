// Package queue is a small in-process worker pool that runs render jobs off the
// request path. Jobs run under the server's lifetime context, not the request's,
// so a returned HTTP response does not cancel an in-flight render.
//
// NOTE: this is intentionally in-process (matching the original multiprocessing
// pool). For durability across restarts/retries, back it with Redis/BullMQ or a
// DB-backed queue later — the Submit/Job boundary is designed to make that swap
// localized.
package queue

import (
	"context"
	"log"
	"sync"
)

// Job is a unit of background work.
type Job func(ctx context.Context)

type Queue struct {
	jobs    chan Job
	workers int
	wg      sync.WaitGroup
}

// New creates a queue with the given worker count and buffer capacity.
func New(workers, buffer int) *Queue {
	if workers < 1 {
		workers = 1
	}
	if buffer < workers {
		buffer = workers
	}
	return &Queue{jobs: make(chan Job, buffer), workers: workers}
}

// Start launches the workers. They drain until the jobs channel is closed.
func (q *Queue) Start(ctx context.Context) {
	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go func(id int) {
			defer q.wg.Done()
			for job := range q.jobs {
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("worker %d recovered from panic: %v", id, r)
						}
					}()
					job(ctx)
				}()
			}
		}(i)
	}
}

// Submit enqueues a job without blocking. It returns false if the buffer is full.
func (q *Queue) Submit(job Job) bool {
	select {
	case q.jobs <- job:
		return true
	default:
		return false
	}
}

// Shutdown stops accepting jobs and waits for in-flight work to finish.
func (q *Queue) Shutdown() {
	close(q.jobs)
	q.wg.Wait()
}
