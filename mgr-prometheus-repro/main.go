package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Stats struct {
	totalRequests  atomic.Int64
	totalErrors    atomic.Int64
	totalTimeouts  atomic.Int64
	totalBytes     atomic.Int64
	consecutiveTO  atomic.Int64
	maxConsecTO    atomic.Int64
	latencies      []time.Duration
	mu             sync.Mutex
}

func (s *Stats) recordLatency(d time.Duration) {
	s.mu.Lock()
	s.latencies = append(s.latencies, d)
	s.mu.Unlock()
}

func (s *Stats) p99Latency() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.latencies) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(s.latencies))
	copy(sorted, s.latencies)
	// simple selection for p99
	idx := int(math.Ceil(float64(len(sorted))*0.99)) - 1
	if idx < 0 {
		idx = 0
	}
	// partial sort: find the idx-th element
	for i := 0; i <= idx; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j] < sorted[i] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted[idx]
}

type MemSample struct {
	ts  time.Time
	rss float64 // bytes, read from /proc/<pid>/statm
}

func findMgrPid() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(comm)) == "ceph-mgr" {
			return pid
		}
	}
	return 0
}

func readProcRSS(pid int) float64 {
	if pid <= 0 {
		return 0
	}
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0
	}
	pages, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0
	}
	return float64(pages) * 4096 // page size = 4KB
}

func scrapeWorker(ctx context.Context, client *http.Client, endpoint string, stats *Stats, timeout time.Duration) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		req, err := http.NewRequestWithContext(reqCtx, "GET", endpoint, nil)
		if err != nil {
			cancel()
			stats.totalErrors.Add(1)
			continue
		}

		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)
		cancel()

		stats.totalRequests.Add(1)

		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if os.IsTimeout(err) || strings.Contains(err.Error(), "context deadline exceeded") {
				stats.totalTimeouts.Add(1)
				consec := stats.consecutiveTO.Add(1)
				for {
					cur := stats.maxConsecTO.Load()
					if consec <= cur || stats.maxConsecTO.CompareAndSwap(cur, consec) {
						break
					}
				}
			} else {
				stats.totalErrors.Add(1)
			}
			continue
		}

		stats.consecutiveTO.Store(0)
		stats.recordLatency(elapsed)

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		stats.totalBytes.Add(int64(len(body)))
	}
}

func memoryMonitor(ctx context.Context, pid int, interval time.Duration, samples *[]MemSample, mu *sync.Mutex) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		rss := readProcRSS(pid)
		if rss > 0 {
			mu.Lock()
			*samples = append(*samples, MemSample{ts: time.Now(), rss: rss})
			mu.Unlock()
		}
	}
}

func formatBytes(b float64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", b/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", b/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", b/float64(1<<10))
	default:
		return fmt.Sprintf("%.0f B", b)
	}
}

func main() {
	endpoint := flag.String("endpoint", "http://localhost:9283/metrics", "mgr prometheus endpoint URL")
	workers := flag.Int("workers", 16, "number of concurrent scrape workers")
	duration := flag.Duration("duration", 10*time.Minute, "test duration")
	timeout := flag.Duration("timeout", 30*time.Second, "per-request timeout (deadlock detection threshold)")
	memInterval := flag.Duration("mem-interval", 5*time.Second, "memory sampling interval")
	deadlockThresh := flag.Int("deadlock-consec", 5, "consecutive timeouts to declare deadlock")
	mgrPid := flag.Int("mgr-pid", 0, "ceph-mgr PID (auto-detect if 0)")
	flag.Parse()

	log.SetFlags(log.Ltime | log.Lmicroseconds)
	log.Printf("=== mgr prometheus OOM/deadlock reproducer ===")
	log.Printf("endpoint:    %s", *endpoint)
	log.Printf("workers:     %d", *workers)
	log.Printf("duration:    %s", *duration)
	log.Printf("timeout:     %s", *timeout)
	log.Printf("mem-interval:%s", *memInterval)

	pid := *mgrPid
	if pid == 0 {
		pid = findMgrPid()
	}
	if pid > 0 {
		log.Printf("mgr PID:     %d (monitoring /proc/%d/statm)", pid, pid)
	} else {
		log.Printf("mgr PID:     not found (memory monitoring disabled)")
		log.Printf("  use -mgr-pid=<PID> or run on the same host as ceph-mgr")
	}
	log.Printf("")
	log.Printf("Bug 1 (TTLCache leak): monitor RSS growth over time")
	log.Printf("Bug 2 (deadlock):      detect consecutive timeouts >= %d", *deadlockThresh)
	log.Printf("")

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			log.Printf("interrupted, stopping...")
			cancel()
			// Force exit after 5s if workers don't finish
			time.Sleep(5 * time.Second)
			log.Printf("force exit (workers still running after 5s)")
			os.Exit(130) // 128 + SIGINT
		case <-ctx.Done():
		}
	}()

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        *workers * 2,
			MaxIdleConnsPerHost: *workers * 2,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	stats := &Stats{}
	var memSamples []MemSample
	var memMu sync.Mutex

	// Start memory monitor
	if pid > 0 {
		go memoryMonitor(ctx, pid, *memInterval, &memSamples, &memMu)
	}

	// Start scrape workers
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			scrapeWorker(ctx, client, *endpoint, stats, *timeout)
		}()
	}

	// Progress reporter
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				memMu.Lock()
				var rssStr string
				if len(memSamples) > 0 {
					rssStr = formatBytes(memSamples[len(memSamples)-1].rss)
				} else {
					rssStr = "N/A"
				}
				memMu.Unlock()
				log.Printf("[progress] requests=%d errors=%d timeouts=%d consec_to=%d rss=%s",
					stats.totalRequests.Load(),
					stats.totalErrors.Load(),
					stats.totalTimeouts.Load(),
					stats.consecutiveTO.Load(),
					rssStr)

				if stats.maxConsecTO.Load() >= int64(*deadlockThresh) {
					log.Printf("*** DEADLOCK DETECTED: %d consecutive timeouts ***",
						stats.maxConsecTO.Load())
				}
			}
		}
	}()

	wg.Wait()
	printReport(stats, &memSamples, &memMu, *deadlockThresh, pid)
}

func printReport(stats *Stats, memSamples *[]MemSample, memMu *sync.Mutex, deadlockThresh int, pid int) {
	log.Printf("")
	log.Printf("=== RESULTS ===")
	log.Printf("")
	log.Printf("Requests:    %d", stats.totalRequests.Load())
	log.Printf("Errors:      %d", stats.totalErrors.Load())
	log.Printf("Timeouts:    %d", stats.totalTimeouts.Load())
	log.Printf("Max consec timeouts: %d", stats.maxConsecTO.Load())
	log.Printf("Data received: %s", formatBytes(float64(stats.totalBytes.Load())))
	log.Printf("P99 latency: %s", stats.p99Latency())
	log.Printf("")

	memMu.Lock()
	samples := make([]MemSample, len(*memSamples))
	copy(samples, *memSamples)
	memMu.Unlock()

	// Memory analysis
	if len(samples) >= 2 {
		first := samples[0]
		last := samples[len(samples)-1]
		growth := last.rss - first.rss
		elapsed := last.ts.Sub(first.ts)
		rate := growth / elapsed.Seconds()

		log.Printf("--- Memory (mgr RSS via /proc/%d/statm) ---", pid)
		log.Printf("First sample: %s", formatBytes(first.rss))
		log.Printf("Last sample:  %s", formatBytes(last.rss))
		log.Printf("Growth:       %s", formatBytes(growth))
		log.Printf("Duration:     %s", elapsed.Round(time.Second))
		if rate > 0 {
			log.Printf("Leak rate:    %s/min", formatBytes(rate*60))
		}
		log.Printf("")

		if growth > 50*1024*1024 {
			log.Printf("*** BUG 1 LIKELY: RSS grew by %s (>50MB) ***", formatBytes(growth))
			log.Printf("    TTLCache PyObject leak: objects are cached but never Py_DECREF'd")
			log.Printf("    on eviction, causing unbounded memory growth.")
		} else if growth > 10*1024*1024 {
			log.Printf("NOTE: RSS grew by %s — moderate growth, may need longer run", formatBytes(growth))
		} else {
			log.Printf("RSS growth within normal range (%s)", formatBytes(growth))
		}
	} else {
		log.Printf("--- Memory ---")
		log.Printf("Insufficient samples (mgr PID not found or not accessible)")
		log.Printf("  Run on the same host as ceph-mgr, or specify -mgr-pid=<PID>")
	}

	log.Printf("")

	// Deadlock analysis
	if stats.maxConsecTO.Load() >= int64(deadlockThresh) {
		log.Printf("*** BUG 2 CONFIRMED: deadlock detected (%d consecutive timeouts) ***",
			stats.maxConsecTO.Load())
		log.Printf("    HealthHistory.check() acquires Lock then calls save() which")
		log.Printf("    also tries to acquire the same Lock → deadlock.")
		log.Printf("    Fix: use RLock (reentrant) instead of Lock.")
	} else if stats.totalTimeouts.Load() > 0 {
		log.Printf("Some timeouts observed (%d) but no sustained deadlock pattern.",
			stats.totalTimeouts.Load())
	} else {
		log.Printf("No deadlock detected (0 timeouts).")
	}

	log.Printf("")
	log.Printf("=== END ===")

	// Exit code
	if stats.maxConsecTO.Load() >= int64(deadlockThresh) {
		os.Exit(2) // deadlock
	}
	if len(samples) >= 2 && samples[len(samples)-1].rss-samples[0].rss > 50*1024*1024 {
		os.Exit(1) // leak
	}
}
