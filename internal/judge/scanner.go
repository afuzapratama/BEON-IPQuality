package judge

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lfrfrfr/beon-ipquality/pkg/logger"
)

// ScanResult contains the results of active scanning
type ScanResult struct {
	IP            string        `json:"ip"`
	IsProxy       bool          `json:"is_proxy"`
	IsSOCKS4      bool          `json:"is_socks4"`
	IsSOCKS5      bool          `json:"is_socks5"`
	IsHTTPProxy   bool          `json:"is_http_proxy"`
	IsHTTPConnect bool          `json:"is_http_connect"`
	OpenPorts     []int         `json:"open_ports"`
	ProxyPorts    []int         `json:"proxy_ports"`
	Headers       *HeaderResult `json:"headers,omitempty"`
	ScanTime      float64       `json:"scan_time_ms"`
	Error         string        `json:"error,omitempty"`
}

// HeaderResult contains HTTP header inspection results
type HeaderResult struct {
	RevealingHeaders map[string]string `json:"revealing_headers,omitempty"`
	IsTransparent    bool              `json:"is_transparent"`
	IsAnonymous      bool              `json:"is_anonymous"`
	IsElite          bool              `json:"is_elite"`
}

// Scanner performs active proxy detection
type Scanner struct {
	timeout    time.Duration
	proxyPorts []int
	httpPorts  []int
	socksPort  []int
	maxWorkers int
	httpClient *http.Client
	externalIP string
}

// ScannerConfig holds scanner configuration
type ScannerConfig struct {
	Timeout    time.Duration
	MaxWorkers int
	ExternalIP string // Our external IP for header detection
}

// DefaultProxyPorts common proxy ports to scan
var DefaultProxyPorts = []int{
	80, 81, 83, 88, // HTTP
	443,                    // HTTPS
	3128, 8080, 8081, 8888, // HTTP Proxy
	8118,       // Privoxy
	1080,       // SOCKS
	9050, 9051, // Tor
}

// DefaultSOCKSPorts common SOCKS ports
var DefaultSOCKSPorts = []int{1080, 1081, 1082, 9050, 9051}

// DefaultHTTPPorts common HTTP proxy ports
var DefaultHTTPPorts = []int{80, 81, 3128, 8080, 8081, 8888, 8118}

// RevealingHeaders headers that reveal proxy usage
var RevealingHeaders = []string{
	"X-Forwarded-For",
	"X-Real-IP",
	"X-Proxy-ID",
	"X-Forwarded-Proto",
	"X-Forwarded-Host",
	"Via",
	"Forwarded",
	"X-Client-IP",
	"X-Originating-IP",
	"CF-Connecting-IP",
	"True-Client-IP",
	"X-Cluster-Client-IP",
}

// NewScanner creates a new proxy scanner
func NewScanner(cfg ScannerConfig) *Scanner {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	maxWorkers := cfg.MaxWorkers
	if maxWorkers == 0 {
		maxWorkers = 10
	}

	return &Scanner{
		timeout:    timeout,
		proxyPorts: DefaultProxyPorts,
		httpPorts:  DefaultHTTPPorts,
		socksPort:  DefaultSOCKSPorts,
		maxWorkers: maxWorkers,
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects
			},
		},
		externalIP: cfg.ExternalIP,
	}
}

// Scan performs a comprehensive scan on an IP
func (s *Scanner) Scan(ctx context.Context, ip string) *ScanResult {
	start := time.Now()
	result := &ScanResult{
		IP:         ip,
		OpenPorts:  []int{},
		ProxyPorts: []int{},
	}

	// Port scan first
	openPorts := s.scanPorts(ctx, ip, s.proxyPorts)
	result.OpenPorts = openPorts

	if len(openPorts) == 0 {
		result.ScanTime = float64(time.Since(start).Milliseconds())
		return result
	}

	// Check each open port for proxy
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, port := range openPorts {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()

			if s.isSOCKS5(ctx, ip, p) {
				mu.Lock()
				result.IsSOCKS5 = true
				result.IsProxy = true
				result.ProxyPorts = append(result.ProxyPorts, p)
				mu.Unlock()
				return
			}

			if s.isSOCKS4(ctx, ip, p) {
				mu.Lock()
				result.IsSOCKS4 = true
				result.IsProxy = true
				result.ProxyPorts = append(result.ProxyPorts, p)
				mu.Unlock()
				return
			}

			if s.isHTTPProxy(ctx, ip, p) {
				mu.Lock()
				result.IsHTTPProxy = true
				result.IsProxy = true
				result.ProxyPorts = append(result.ProxyPorts, p)
				mu.Unlock()
				return
			}

			if s.isHTTPConnect(ctx, ip, p) {
				mu.Lock()
				result.IsHTTPConnect = true
				result.IsProxy = true
				result.ProxyPorts = append(result.ProxyPorts, p)
				mu.Unlock()
				return
			}
		}(port)
	}

	wg.Wait()
	result.ScanTime = float64(time.Since(start).Milliseconds())
	return result
}

// QuickScan performs a fast scan on common ports only
func (s *Scanner) QuickScan(ctx context.Context, ip string) *ScanResult {
	start := time.Now()
	result := &ScanResult{
		IP:         ip,
		OpenPorts:  []int{},
		ProxyPorts: []int{},
	}

	// Only check most common proxy ports
	quickPorts := []int{1080, 3128, 8080, 8888}
	openPorts := s.scanPorts(ctx, ip, quickPorts)
	result.OpenPorts = openPorts

	for _, port := range openPorts {
		if s.isSOCKS5(ctx, ip, port) {
			result.IsSOCKS5 = true
			result.IsProxy = true
			result.ProxyPorts = append(result.ProxyPorts, port)
		} else if s.isHTTPProxy(ctx, ip, port) {
			result.IsHTTPProxy = true
			result.IsProxy = true
			result.ProxyPorts = append(result.ProxyPorts, port)
		}
	}

	result.ScanTime = float64(time.Since(start).Milliseconds())
	return result
}

// scanPorts scans multiple ports concurrently
func (s *Scanner) scanPorts(ctx context.Context, ip string, ports []int) []int {
	var openPorts []int
	var mu sync.Mutex
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, s.maxWorkers)

	for _, port := range ports {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			if s.isPortOpen(ctx, ip, p) {
				mu.Lock()
				openPorts = append(openPorts, p)
				mu.Unlock()
			}
		}(port)
	}

	wg.Wait()
	return openPorts
}

// isPortOpen checks if a port is open
func (s *Scanner) isPortOpen(ctx context.Context, ip string, port int) bool {
	addr := fmt.Sprintf("%s:%d", ip, port)

	dialer := &net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// isSOCKS5 checks if port is running SOCKS5
func (s *Scanner) isSOCKS5(ctx context.Context, ip string, port int) bool {
	addr := fmt.Sprintf("%s:%d", ip, port)

	dialer := &net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()

	// Set deadline
	conn.SetDeadline(time.Now().Add(s.timeout))

	// SOCKS5 handshake: send version + auth methods
	// Version 5, 1 method, no auth (0x00)
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		return false
	}

	// Read response
	buf := make([]byte, 2)
	n, err := conn.Read(buf)
	if err != nil || n < 2 {
		return false
	}

	// Check if response is SOCKS5 (version 5, method accepted)
	return buf[0] == 0x05 && buf[1] == 0x00
}

// isSOCKS4 checks if port is running SOCKS4
func (s *Scanner) isSOCKS4(ctx context.Context, ip string, port int) bool {
	addr := fmt.Sprintf("%s:%d", ip, port)

	dialer := &net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(s.timeout))

	// SOCKS4 connect request to google.com:80
	// VN=4, CD=1 (connect), DSTPORT=80, DSTIP=142.250.185.206 (google), USERID=null
	request := []byte{
		0x04, 0x01, // Version 4, Connect command
		0x00, 0x50, // Port 80
		0x8e, 0xfa, 0xb9, 0xce, // 142.250.185.206
		0x00, // Null terminated userid
	}

	_, err = conn.Write(request)
	if err != nil {
		return false
	}

	buf := make([]byte, 8)
	n, err := conn.Read(buf)
	if err != nil || n < 2 {
		return false
	}

	// Check response: VN=0, CD=90 (request granted)
	return buf[0] == 0x00 && buf[1] == 0x5a
}

// isHTTPProxy checks if port is running HTTP proxy
func (s *Scanner) isHTTPProxy(ctx context.Context, ip string, port int) bool {
	addr := fmt.Sprintf("%s:%d", ip, port)

	dialer := &net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(s.timeout))

	// Send HTTP proxy request
	request := "GET http://www.google.com/ HTTP/1.1\r\nHost: www.google.com\r\nConnection: close\r\n\r\n"
	_, err = conn.Write([]byte(request))
	if err != nil {
		return false
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return false
	}

	response := string(buf[:n])
	// Check if we got a valid HTTP response (proxy forwarded our request)
	return strings.HasPrefix(response, "HTTP/") &&
		(strings.Contains(response, "200") ||
			strings.Contains(response, "301") ||
			strings.Contains(response, "302") ||
			strings.Contains(response, "403"))
}

// isHTTPConnect checks if port supports HTTP CONNECT
func (s *Scanner) isHTTPConnect(ctx context.Context, ip string, port int) bool {
	addr := fmt.Sprintf("%s:%d", ip, port)

	dialer := &net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(s.timeout))

	// Send CONNECT request
	request := "CONNECT www.google.com:443 HTTP/1.1\r\nHost: www.google.com:443\r\n\r\n"
	_, err = conn.Write([]byte(request))
	if err != nil {
		return false
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return false
	}

	response := string(buf[:n])
	// Check for successful CONNECT response
	return strings.HasPrefix(response, "HTTP/") && strings.Contains(response, "200")
}

// InspectHeaders inspects HTTP headers from a request to detect proxy
func (s *Scanner) InspectHeaders(headers map[string][]string, clientIP string) *HeaderResult {
	result := &HeaderResult{
		RevealingHeaders: make(map[string]string),
		IsElite:          true, // Assume elite until proven otherwise
	}

	for _, header := range RevealingHeaders {
		if values, ok := headers[header]; ok && len(values) > 0 {
			result.RevealingHeaders[header] = values[0]
			result.IsElite = false

			// Check if header reveals real IP
			for _, val := range values {
				if strings.Contains(val, clientIP) || strings.Contains(val, s.externalIP) {
					result.IsTransparent = true
				}
			}
		}
	}

	// Determine anonymity level
	if result.IsTransparent {
		result.IsAnonymous = false
	} else if len(result.RevealingHeaders) > 0 {
		result.IsAnonymous = true
	}

	return result
}

// ScanAsync performs scan asynchronously and returns channel
func (s *Scanner) ScanAsync(ctx context.Context, ip string) <-chan *ScanResult {
	ch := make(chan *ScanResult, 1)
	go func() {
		defer close(ch)
		result := s.Scan(ctx, ip)
		select {
		case ch <- result:
		case <-ctx.Done():
		}
	}()
	return ch
}

// BatchScan scans multiple IPs
func (s *Scanner) BatchScan(ctx context.Context, ips []string) []*ScanResult {
	results := make([]*ScanResult, len(ips))
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, s.maxWorkers)

	for i, ip := range ips {
		wg.Add(1)
		go func(idx int, ipAddr string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			results[idx] = s.Scan(ctx, ipAddr)
		}(i, ip)
	}

	wg.Wait()
	return results
}

// GetExternalIP gets our external IP address
func GetExternalIP() (string, error) {
	services := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
		"https://icanhazip.com",
	}

	client := &http.Client{Timeout: 5 * time.Second}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		ip := strings.TrimSpace(string(body))
		if net.ParseIP(ip) != nil {
			return ip, nil
		}
	}

	return "", fmt.Errorf("failed to get external IP")
}

func init() {
	// Get our external IP on startup
	if ip, err := GetExternalIP(); err == nil {
		logger.Info(fmt.Sprintf("External IP detected: %s", ip))
	}
}
