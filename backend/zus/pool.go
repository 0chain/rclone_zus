// Custom upload pool for rclone-zus that mirrors zs3server's batchUploadWorker
// design: 1 dispatcher goroutine accumulates ops by (count | bytes | timeout),
// N worker goroutines commit batches in parallel via DoMultiOperation.
//
// Why not rclone's stdlib batcher? That one has a single commit goroutine,
// so commitBatch calls serialize even when multiple ops have been buffered.
// zs3server's 5-worker pool was the measured-fast path; this replicates it
// with adaptive sizing (count + byte + time, whichever fires first).
package zus

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0chain/gosdk/zboxcore/sdk"
)

// poolDebug is set via RCLONE_ZUS_POOL_DEBUG=1. When true, the pool logs
// submit / dispatcher / worker events. Keep cheap checks on hot path.
var poolDebug = os.Getenv("RCLONE_ZUS_POOL_DEBUG") == "1"

type poolItem struct {
	op   sdk.OperationRequest
	size int64
}

type uploadPool struct {
	ctx           context.Context
	cancel        context.CancelFunc
	alloc         *sdk.Allocation
	opsChan       chan poolItem
	batchChan     chan []poolItem
	maxBatchCount int
	maxBatchBytes int64
	waitDur       time.Duration
	workers       int
	wg            sync.WaitGroup
	inflight      sync.WaitGroup
	errOnce       sync.Once
	firstErr      atomic.Value // stores error
	shutdownOnce  sync.Once
	shutdownErr   error
	// counters for debug/logging
	submitted  atomic.Int64
	dispatched atomic.Int64
	committed  atomic.Int64
}

func newUploadPool(alloc *sdk.Allocation, workers, batchCount int, batchBytes int64, waitMs int) *uploadPool {
	ctx, cancel := context.WithCancel(context.Background())
	if workers < 1 {
		workers = 1
	}
	if batchCount < 1 {
		batchCount = 1
	}
	if waitMs < 1 {
		waitMs = 1
	}
	p := &uploadPool{
		ctx:           ctx,
		cancel:        cancel,
		alloc:         alloc,
		opsChan:       make(chan poolItem, workers*batchCount*2),
		batchChan:     make(chan []poolItem, workers),
		maxBatchCount: batchCount,
		maxBatchBytes: batchBytes,
		waitDur:       time.Duration(waitMs) * time.Millisecond,
		workers:       workers,
	}
	log.Printf("[zus-pool] start: workers=%d batchCount=%d batchBytes=%d waitMs=%d debug=%v",
		workers, batchCount, batchBytes, waitMs, poolDebug)
	p.wg.Add(1)
	go p.dispatcher()
	for i := 0; i < workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	return p
}

func (p *uploadPool) dispatcher() {
	defer p.wg.Done()
	defer close(p.batchChan)
	var batch []poolItem
	var bytes int64
	timer := time.NewTimer(p.waitDur)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	flush := func(reason string) {
		if len(batch) == 0 {
			return
		}
		b := batch
		bsz := bytes
		batch = nil
		bytes = 0
		if poolDebug {
			log.Printf("[zus-pool] dispatch flush: reason=%s count=%d bytes=%d", reason, len(b), bsz)
		}
		p.dispatched.Add(int64(len(b)))
		// Use a send that respects ctx so we don't block forever if workers died.
		select {
		case p.batchChan <- b:
		case <-p.ctx.Done():
			// Pool shutting down; caller will drain inflight separately.
			// Decrement inflight for any ops in this batch since they'll never commit.
			for range b {
				p.inflight.Done()
			}
		}
	}
	// Per-op threshold at which we consider an op "large" and flush the
	// batch immediately after adding it. Prevents one worker from getting
	// a huge solo batch while other workers idle. Half of batchBytes is a
	// good heuristic: it lets big files spread across workers.
	largeOpBytes := p.maxBatchBytes / 2
	if largeOpBytes <= 0 {
		largeOpBytes = 64 * 1024 * 1024 // 64 MiB
	}
	for {
		select {
		case <-p.ctx.Done():
			flush("ctx-done")
			return
		case item, ok := <-p.opsChan:
			if !ok {
				flush("ops-closed")
				return
			}
			batch = append(batch, item)
			bytes += item.size
			if len(batch) == 1 {
				// Drain any pending fire before resetting to avoid a
				// spurious <-timer.C right after Reset.
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(p.waitDur)
			}
			// Flush triggers (whichever hits first):
			//   - count reached
			//   - bytes reached
			//   - single op is "large" (fan out big files to separate workers)
			if len(batch) >= p.maxBatchCount ||
				(p.maxBatchBytes > 0 && bytes >= p.maxBatchBytes) ||
				item.size >= largeOpBytes {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				reason := "count"
				if item.size >= largeOpBytes {
					reason = "large-op"
				} else if p.maxBatchBytes > 0 && bytes >= p.maxBatchBytes {
					reason = "bytes"
				}
				flush(reason)
			}
		case <-timer.C:
			flush("timer")
		}
	}
}

func (p *uploadPool) worker(id int) {
	defer p.wg.Done()
	for batch := range p.batchChan {
		p.commitOne(id, batch)
	}
}

// commitOne runs a single DoMultiOperation for a batch, with panic recovery
// so inflight counters drain even if the SDK panics. Without this a panic
// leaves inflight > 0 and Shutdown() hangs forever.
func (p *uploadPool) commitOne(id int, batch []poolItem) {
	var bsz int64
	for _, b := range batch {
		bsz += b.size
	}
	start := time.Now()
	if poolDebug {
		log.Printf("[zus-pool] worker=%d commit start: count=%d bytes=%d", id, len(batch), bsz)
	}
	defer func() {
		// Always release one inflight per op, even on panic, so Shutdown drains.
		if r := recover(); r != nil {
			err := fmt.Errorf("worker=%d panic: %v", id, r)
			p.errOnce.Do(func() { p.firstErr.Store(err) })
			log.Printf("[zus-pool] %v", err)
		}
		for range batch {
			p.inflight.Done()
		}
		p.committed.Add(int64(len(batch)))
		if poolDebug {
			log.Printf("[zus-pool] worker=%d commit done: count=%d bytes=%d dur=%s",
				id, len(batch), bsz, time.Since(start))
		}
	}()
	ops := make([]sdk.OperationRequest, len(batch))
	for i, b := range batch {
		ops[i] = b.op
	}
	err := p.alloc.DoMultiOperation(ops)
	if err != nil {
		p.errOnce.Do(func() { p.firstErr.Store(err) })
		log.Printf("[zus-pool] worker=%d commit error: count=%d bytes=%d err=%v",
			id, len(batch), bsz, err)
	}
}

// submit is non-blocking: it enqueues the op and returns nil so that
// rclone's --transfers=N semaphore releases immediately. The actual commit
// happens asynchronously in workers. This mirrors mc's behavior via
// zs3server (PutObject returns after tmpfs write, background commits).
// Errors surface in Shutdown().
//
// inflight is incremented here and decremented by the worker after commit
// so Shutdown() can wait for full drain.
func (p *uploadPool) submit(op sdk.OperationRequest, size int64) error {
	p.inflight.Add(1)
	p.submitted.Add(1)
	if poolDebug {
		log.Printf("[zus-pool] submit: size=%d submitted=%d dispatched=%d committed=%d",
			size, p.submitted.Load(), p.dispatched.Load(), p.committed.Load())
	}
	select {
	case p.opsChan <- poolItem{op: op, size: size}:
		return nil
	case <-p.ctx.Done():
		p.inflight.Done()
		return p.ctx.Err()
	}
}

// Shutdown drains all pending/in-flight uploads, stops the dispatcher and
// workers, and returns the first commit error observed (if any). Call from
// rclone fs.Shutdown hook so rclone's copy operation reports failures.
// Idempotent — repeat calls return the same result.
func (p *uploadPool) shutdown() error {
	p.shutdownOnce.Do(func() {
		log.Printf("[zus-pool] shutdown begin: submitted=%d dispatched=%d committed=%d",
			p.submitted.Load(), p.dispatched.Load(), p.committed.Load())
		// Close opsChan so dispatcher flushes final partial batch and exits.
		// inflight counts outstanding ops; wait for them to drain before
		// cancelling workers (cancel would otherwise leak ops mid-commit).
		close(p.opsChan)
		p.inflight.Wait()
		p.cancel()
		p.wg.Wait()
		log.Printf("[zus-pool] shutdown end: submitted=%d dispatched=%d committed=%d",
			p.submitted.Load(), p.dispatched.Load(), p.committed.Load())
		if v := p.firstErr.Load(); v != nil {
			if err, ok := v.(error); ok {
				p.shutdownErr = fmt.Errorf("rclone-zus async commit error: %w", err)
			}
		}
	})
	return p.shutdownErr
}
