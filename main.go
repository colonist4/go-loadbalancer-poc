package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type Backend struct {
	Host      string
	Proxy     *httputil.ReverseProxy
	ByteToken float32
	ReqToken  float32

	LastTokenTimeMs int64
	ByteTokenPerMs  float32
	ReqTokenPerMs   float32

	MaxByteToken float32
	MaxReqToken  float32

	mu sync.Mutex
}

func (b *Backend) ConsumeTokenIfPossible(bodyBytes int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now().UnixMilli()
	elapsedMs := now - b.LastTokenTimeMs

	// Refill tokens
	b.ByteToken += float32(elapsedMs) * b.ByteTokenPerMs
	b.ReqToken += float32(elapsedMs) * b.ReqTokenPerMs
	b.LastTokenTimeMs = now

	if b.ByteToken >= float32(bodyBytes) && b.ReqToken >= 1 {
		b.ByteToken -= float32(bodyBytes)
		b.ReqToken -= 1
		return true
	}

	return false
}

type LoadBalancer struct {
	Backends     []*Backend
	BackendCount int64
	ReqIdx       atomic.Int64
	BPMLimit     int
	RPMLimit     int
}

func (lb *LoadBalancer) RegisterBackend(host string, port int) {
	backend := &Backend{
		Host: host,
		Proxy: httputil.NewSingleHostReverseProxy(
			&url.URL{Scheme: "http", Host: fmt.Sprintf("%s:%d", host, port)},
		),
		LastTokenTimeMs: time.Now().UnixMilli(),
		ByteTokenPerMs:  float32(lb.BPMLimit) / 60 / 1000,
		ReqTokenPerMs:   float32(lb.RPMLimit) / 60 / 1000,
		MaxByteToken:    float32(lb.BPMLimit),
		MaxReqToken:     float32(lb.RPMLimit),
	}
	lb.Backends = append(lb.Backends, backend)
	lb.BackendCount += 1
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	var backOffTime = 100 * time.Millisecond
	var backOffCount = 0

	for {
		var ReqIdx = lb.ReqIdx.Add(1)
		backend := lb.Backends[ReqIdx%lb.BackendCount]

		if backend.ConsumeTokenIfPossible(int(r.ContentLength)) {
			backend.Proxy.ServeHTTP(w, r)
			return
		}

		if backOffCount > 5 {
			http.Error(w, "Rate limit exceeded", http.StatusServiceUnavailable)
			return
		}

		time.Sleep(backOffTime)
		backOffTime *= 2
		if backOffTime > 1*time.Second {
			backOffTime = 1 * time.Second
		}
		backOffCount += 1
	}
}

func NewLoadBalancer() *LoadBalancer {
	return &LoadBalancer{
		Backends:     make([]*Backend, 0),
		BackendCount: 0,
		ReqIdx:       atomic.Int64{},
		BPMLimit:     1024 * 1024,
		RPMLimit:     60,
	}
}

func main() {
	isBackendMode := flag.Bool("backend", false, "Run as backend server")
	port := flag.Int("port", 8080, "Server port")
	flag.Parse()

	if *isBackendMode {
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Request from %s\n", r.RemoteAddr)
			w.Write([]byte("Hello, World!"))
		})

		if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
			log.Fatal(err)
		}
	} else {
		var lb = NewLoadBalancer()
		lb.RegisterBackend("localhost", 8081)
		lb.RegisterBackend("localhost", 8082)

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			lb.ServeHTTP(w, r)
		})

		if err := http.ListenAndServe(fmt.Sprintf(":%d", *port), nil); err != nil {
			log.Fatal(err)
		}
	}

}
