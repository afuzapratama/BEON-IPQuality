// loadcheck measures a private, isolated API instance without logging keys.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"sync"
	"time"
)

func main() {
	base := flag.String("url", "http://127.0.0.1:18080", "loopback benchmark API")
	concurrency := flag.Int("concurrency", 8, "concurrent clients (1..64)")
	duration := flag.Duration("duration", 5*time.Second, "test duration (max 30s)")
	batch := flag.Bool("batch", false, "send a batch of four public IPv4/IPv6 addresses")
	flag.Parse()
	target, err := url.Parse(*base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid URL")
		os.Exit(2)
	}
	ip := net.ParseIP(target.Hostname())
	if err != nil || ip == nil || !ip.IsLoopback() || *concurrency < 1 || *concurrency > 64 || *duration <= 0 || *duration > 30*time.Second {
		fmt.Fprintln(os.Stderr, "Use a loopback URL, concurrency 1..64 and duration <=30s")
		os.Exit(2)
	}
	key := os.Getenv("BEON_BENCH_API_KEY")
	if key == "" {
		fmt.Fprintln(os.Stderr, "BEON_BENCH_API_KEY required")
		os.Exit(2)
	}
	transport := &http.Transport{MaxIdleConns: *concurrency, MaxIdleConnsPerHost: *concurrency, MaxConnsPerHost: *concurrency}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	defer transport.CloseIdleConnections()
	var mu sync.Mutex
	var workers sync.WaitGroup
	latencies := make([]int64, 0, 10000)
	statuses := make(map[int]int)
	failures := 0
	start := time.Now()
	end := start.Add(*duration)
	for i := 0; i < *concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for time.Now().Before(end) {
				method, path := "GET", "/api/v1/check/8.8.8.8"
				var body io.Reader
				if *batch {
					method, path = "POST", "/api/v1/check/batch"
					body = bytes.NewBufferString(`{"ips":["8.8.8.8","1.1.1.1","9.9.9.9","2001:4860:4860::8888"]}`)
				}
				req, _ := http.NewRequest(method, *base+path, body)
				req.Header.Set("X-API-Key", key)
				req.Header.Set("Content-Type", "application/json")
				began := time.Now()
				response, err := client.Do(req)
				code := 0
				if err == nil {
					code = response.StatusCode
					_, err = io.Copy(io.Discard, response.Body)
					response.Body.Close()
				}
				latency := time.Since(began).Nanoseconds()
				mu.Lock()
				statuses[code]++
				if err != nil || code != 200 {
					failures++
				} else {
					latencies = append(latencies, latency)
				}
				mu.Unlock()
			}
		}()
	}
	workers.Wait()
	elapsed := time.Since(start).Seconds()
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(p float64) float64 {
		if len(latencies) == 0 {
			return 0
		}
		return float64(latencies[int(float64(len(latencies)-1)*p)]) / 1e6
	}
	json.NewEncoder(os.Stdout).Encode(map[string]any{"concurrency": *concurrency, "batch": *batch, "duration_seconds": elapsed, "requests": len(latencies) + failures, "success": len(latencies), "errors": failures, "statuses": statuses, "successful_requests_per_second": float64(len(latencies)) / elapsed, "p50_ms": percentile(.50), "p95_ms": percentile(.95), "p99_ms": percentile(.99)})
	if failures > 0 {
		os.Exit(1)
	}
}
