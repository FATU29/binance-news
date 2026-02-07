package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"crawl-news/pkg/logger"
)

// SentimentJob represents a single sentiment analysis job in the queue
type SentimentJob struct {
	NewsID    string
	EnqueueAt time.Time
	Retries   int
}

// QueueStats holds real-time queue statistics
type QueueStats struct {
	Pending    int    `json:"pending"`
	Processing int    `json:"processing"`
	Completed  int64  `json:"completed"`
	Failed     int64  `json:"failed"`
	Retried    int64  `json:"retried"`
	Workers    int    `json:"workers"`
	IsRunning  bool   `json:"is_running"`
	Uptime     string `json:"uptime"`
}

// SentimentQueue is a buffered, persistent-in-memory queue that processes
// sentiment analysis jobs with controlled concurrency and automatic retries.
type SentimentQueue struct {
	aiService  *AIService
	jobChan    chan SentimentJob
	workers    int
	maxRetries int
	wg         sync.WaitGroup
	cancel     context.CancelFunc
	ctx        context.Context

	// Stats (atomic for lock-free reads)
	completed  atomic.Int64
	failed     atomic.Int64
	retried    atomic.Int64
	processing atomic.Int32
	startedAt  time.Time
	isRunning  bool
	mu         sync.RWMutex
}

// NewSentimentQueue creates a new queue with the given buffer size and worker count.
//   - queueSize: how many jobs can wait in the channel buffer
//   - workers:   number of concurrent goroutines that consume the queue
//   - maxRetries: how many times a failed job is re-enqueued
func NewSentimentQueue(aiService *AIService, queueSize, workers, maxRetries int) *SentimentQueue {
	if queueSize <= 0 {
		queueSize = 500
	}
	if workers <= 0 {
		workers = 3
	}
	if maxRetries <= 0 {
		maxRetries = 2
	}

	return &SentimentQueue{
		aiService:  aiService,
		jobChan:    make(chan SentimentJob, queueSize),
		workers:    workers,
		maxRetries: maxRetries,
	}
}

// Start launches the worker pool. Safe to call only once.
func (q *SentimentQueue) Start() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.isRunning {
		logger.Warn("Sentiment queue is already running")
		return
	}

	q.ctx, q.cancel = context.WithCancel(context.Background())
	q.startedAt = time.Now()
	q.isRunning = true

	for i := 0; i < q.workers; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}

	logger.Info("🚀 Sentiment queue started: %d workers, buffer=%d, maxRetries=%d",
		q.workers, cap(q.jobChan), q.maxRetries)
}

// Stop gracefully drains the queue and waits for in-flight jobs.
func (q *SentimentQueue) Stop() {
	q.mu.Lock()
	if !q.isRunning {
		q.mu.Unlock()
		return
	}
	q.isRunning = false
	q.mu.Unlock()

	logger.Info("Stopping sentiment queue (draining %d pending jobs)…", len(q.jobChan))
	q.cancel()  // signal workers to exit
	q.wg.Wait() // wait for in-flight jobs
	logger.Info("Sentiment queue stopped. completed=%d failed=%d", q.completed.Load(), q.failed.Load())
}

// Enqueue adds a news article to the sentiment analysis queue.
// It is non-blocking; if the buffer is full it logs a warning and drops the job.
func (q *SentimentQueue) Enqueue(newsID string) {
	q.mu.RLock()
	running := q.isRunning
	q.mu.RUnlock()
	if !running {
		logger.Warn("Sentiment queue not running – dropping job for news %s", newsID)
		return
	}

	job := SentimentJob{
		NewsID:    newsID,
		EnqueueAt: time.Now(),
		Retries:   0,
	}

	select {
	case q.jobChan <- job:
		logger.Debug("Enqueued sentiment job for news %s (pending=%d)", newsID, len(q.jobChan))
	default:
		logger.Warn("Sentiment queue full (%d) – dropping job for news %s", cap(q.jobChan), newsID)
	}
}

// Stats returns a snapshot of queue statistics.
func (q *SentimentQueue) Stats() QueueStats {
	q.mu.RLock()
	running := q.isRunning
	q.mu.RUnlock()

	uptime := ""
	if running {
		uptime = time.Since(q.startedAt).Truncate(time.Second).String()
	}

	return QueueStats{
		Pending:    len(q.jobChan),
		Processing: int(q.processing.Load()),
		Completed:  q.completed.Load(),
		Failed:     q.failed.Load(),
		Retried:    q.retried.Load(),
		Workers:    q.workers,
		IsRunning:  running,
		Uptime:     uptime,
	}
}

// ---------- internal ----------

func (q *SentimentQueue) worker(id int) {
	defer q.wg.Done()
	logger.Info("Sentiment worker #%d started", id)

	for {
		select {
		case <-q.ctx.Done():
			// Drain remaining jobs in channel before exiting
			for {
				select {
				case job := <-q.jobChan:
					q.processJob(id, job)
				default:
					logger.Info("Sentiment worker #%d stopped", id)
					return
				}
			}

		case job, ok := <-q.jobChan:
			if !ok {
				logger.Info("Sentiment worker #%d: channel closed", id)
				return
			}
			q.processJob(id, job)
		}
	}
}

func (q *SentimentQueue) processJob(workerID int, job SentimentJob) {
	q.processing.Add(1)
	defer q.processing.Add(-1)

	// Per-job timeout
	jobCtx, jobCancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer jobCancel()

	waitTime := time.Since(job.EnqueueAt).Truncate(time.Millisecond)
	logger.Info("Worker #%d processing news %s (waited %s, attempt %d)",
		workerID, job.NewsID, waitTime, job.Retries+1)

	_, err := q.aiService.AnalyzeSentiment(jobCtx, job.NewsID)
	if err != nil {
		if job.Retries < q.maxRetries {
			// Exponential back-off before retry: 2s, 4s
			delay := time.Duration(2<<uint(job.Retries)) * time.Second
			logger.Warn("Worker #%d: sentiment failed for %s (attempt %d/%d, retrying in %s): %v",
				workerID, job.NewsID, job.Retries+1, q.maxRetries+1, delay, err)
			q.retried.Add(1)

			go func(j SentimentJob) {
				time.Sleep(delay)
				j.Retries++
				j.EnqueueAt = time.Now()
				select {
				case q.jobChan <- j:
				default:
					logger.Warn("Queue full on retry – dropping %s", j.NewsID)
					q.failed.Add(1)
				}
			}(job)
		} else {
			logger.Error("Worker #%d: sentiment FAILED permanently for %s after %d attempts: %v",
				workerID, job.NewsID, job.Retries+1, err)
			q.failed.Add(1)
		}
		return
	}

	q.completed.Add(1)
	logger.Info("Worker #%d: ✅ sentiment completed for news %s", workerID, job.NewsID)
}

// EnqueueBatch adds multiple news IDs to the queue.
func (q *SentimentQueue) EnqueueBatch(newsIDs []string) (enqueued int, dropped int) {
	for _, id := range newsIDs {
		job := SentimentJob{
			NewsID:    id,
			EnqueueAt: time.Now(),
			Retries:   0,
		}
		select {
		case q.jobChan <- job:
			enqueued++
		default:
			dropped++
		}
	}
	if dropped > 0 {
		logger.Warn("Batch enqueue: %d enqueued, %d dropped (queue full)", enqueued, dropped)
	} else {
		logger.Info("Batch enqueue: %d jobs enqueued", enqueued)
	}
	return
}

// DrainAndRequeue drains the queue and returns all pending job IDs.
// Useful for diagnostics.
func (q *SentimentQueue) PendingIDs() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()

	ids := make([]string, 0, len(q.jobChan))
	snapshot := len(q.jobChan)
	for i := 0; i < snapshot; i++ {
		select {
		case job := <-q.jobChan:
			ids = append(ids, job.NewsID)
			// Put it back
			q.jobChan <- job
		default:
			break
		}
	}
	return ids
}

// Flush blocks until all currently pending jobs are processed.
func (q *SentimentQueue) Flush() {
	for len(q.jobChan) > 0 || q.processing.Load() > 0 {
		time.Sleep(500 * time.Millisecond)
	}
}

// String returns a human-readable summary.
func (q *SentimentQueue) String() string {
	s := q.Stats()
	return fmt.Sprintf("SentimentQueue{pending=%d processing=%d completed=%d failed=%d workers=%d running=%v}",
		s.Pending, s.Processing, s.Completed, s.Failed, s.Workers, s.IsRunning)
}
