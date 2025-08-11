package httpclient

import (
	"net/http"
	"sync"
	"time"
)

// HTTPClientPool manages a pool of HTTP clients for optimal performance
type HTTPClientPool struct {
	clients chan *http.Client
	factory func() *http.Client
	mu      sync.RWMutex
	closed  bool
}

// NewHTTPClientPool creates a new HTTP client pool
func NewHTTPClientPool(maxClients int) *HTTPClientPool {
	pool := &HTTPClientPool{
		clients: make(chan *http.Client, maxClients),
		factory: createOptimizedHTTPClient,
	}

	// Pre-populate the pool
	for i := 0; i < maxClients; i++ {
		pool.clients <- pool.factory()
	}

	return pool
}

// createOptimizedHTTPClient creates an HTTP client with optimal settings
func createOptimizedHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  false,
			DisableKeepAlives:   false,
		},
	}
}

// Get retrieves an HTTP client from the pool
func (p *HTTPClientPool) Get() *http.Client {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return p.factory()
	}

	select {
	case client := <-p.clients:
		return client
	default:
		// Pool is empty, create a new client
		return p.factory()
	}
}

// Put returns an HTTP client to the pool
func (p *HTTPClientPool) Put(client *http.Client) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed {
		return
	}

	select {
	case p.clients <- client:
		// Successfully returned to pool
	default:
		// Pool is full, discard the client
	}
}

// Close closes the pool and cleans up resources
func (p *HTTPClientPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true
	close(p.clients)
}

// Global pool instance
var (
	globalPool *HTTPClientPool
	once       sync.Once
)

// GetGlobalPool returns the global HTTP client pool
func GetGlobalPool() *HTTPClientPool {
	once.Do(func() {
		globalPool = NewHTTPClientPool(20) // 20 clients in global pool
	})
	return globalPool
}
