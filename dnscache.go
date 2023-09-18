package dnscache

import (
	"context"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"
)

const (
	// cacheSize is initial size of addr and IP list cache map.
	cacheSize = 64

	// defaultFreq is default frequency a resolver refreshes DNS cache.
	defaultFreq = 3 * time.Second

	// defaultLookupTimeout is default timeout for ip lookup
	defaultLookupTimeout = 10 * time.Second
)

type (
	LookupIPFn func(ctx context.Context, host string) ([]net.IP, error)
)

// Resolver is DNS cache resolver which cache DNS resolve results in memory.
type Resolver struct {
	lookupIPFn    LookupIPFn
	lookupTimeout time.Duration

	cache   map[string][]net.IP
	cacheMu sync.RWMutex

	logger *slog.Logger

	refreshTicker *time.Ticker

	closeCh     chan struct{}
	closeFnOnce sync.Once
}

// New initializes DNS cache resolver and starts auto refreshing in a new goroutine.
// To stop refreshing, call `Stop()` function.
func New(freq time.Duration, lookupTimeout time.Duration, options ...Option) (*Resolver, error) {
	if freq <= 0 {
		freq = defaultFreq
	}

	if lookupTimeout <= 0 {
		lookupTimeout = defaultLookupTimeout
	}

	r := &Resolver{
		// lookupIPFn is a wrapper of net.DefaultResolver.LookupIPAddr.
		// This is used to replace lookup function when test.
		lookupIPFn: func(ctx context.Context, host string) ([]net.IP, error) {
			addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}

			ips := make([]net.IP, len(addrs))
			for i, ia := range addrs {
				ips[i] = ia.IP
			}

			return ips, nil
		},
		lookupTimeout: lookupTimeout,
		cache:         make(map[string][]net.IP, cacheSize),
		refreshTicker: time.NewTicker(freq),
		closeCh:       make(chan struct{}),
	}

	for _, p := range options {
		p.apply(r)
	}

	if r.logger == nil {
		r.logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	go func() {
		for {
			select {
			case <-r.refreshTicker.C:
				r.Refresh()
			case <-r.closeCh:
				return
			}
		}
	}()

	return r, nil
}

// LookupIP lookups IP list from DNS server then it saves result in the cache.
// If you want to get result from the cache use `Fetch` function.
func (r *Resolver) LookupIP(ctx context.Context, addr string) ([]net.IP, error) {
	ips, err := r.lookupIPFn(ctx, addr)
	if err != nil {
		return nil, err
	}

	r.cacheMu.Lock()
	r.cache[addr] = ips
	r.cacheMu.Unlock()
	return ips, nil
}

// Fetch fetches IP list from the cache. If IP list of the given addr is not in the cache,
// then it lookups from DNS server by `Lookup` function.
func (r *Resolver) Fetch(ctx context.Context, addr string) ([]net.IP, error) {
	r.cacheMu.RLock()
	ips, ok := r.cache[addr]
	r.cacheMu.RUnlock()
	if ok {
		return ips, nil
	}
	return r.LookupIP(ctx, addr)
}

// Refresh refreshes IP list cache.
func (r *Resolver) Refresh() {
	r.cacheMu.RLock()
	addrs := make([]string, 0, len(r.cache))
	for addr := range r.cache {
		addrs = append(addrs, addr)
	}
	r.cacheMu.RUnlock()

	for _, addr := range addrs {
		ctx, cancelF := context.WithTimeout(context.Background(), r.lookupTimeout)
		if _, err := r.LookupIP(ctx, addr); err != nil {
			r.logger.Warn("failed to refresh DNS cache", "addr", addr, "error", err)
		}
		cancelF()
	}
}

// Stop stops auto refreshing.
func (r *Resolver) Stop() {
	r.closeFnOnce.Do(func() {
		r.refreshTicker.Stop()
		close(r.closeCh)
	})
}
