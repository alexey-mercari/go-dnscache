package dnscache

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var (
	testFreq                 = 1 * time.Second
	testDefaultLookupTimeout = 1 * time.Second
)

func testResolver(t *testing.T, params ...Option) *Resolver {
	t.Helper()
	r, err := New(testFreq, testDefaultLookupTimeout, params...)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	return r
}

func setLookupUpFn(t *testing.T, resolver *Resolver, fn LookupIPFn) {
	t.Helper()
	resolver.cacheMu.Lock()
	defer resolver.cacheMu.Unlock()
	resolver.lookupIPFn = fn
}

func TestNew(t *testing.T) {
	{
		resolver, err := New(testFreq, testDefaultLookupTimeout)
		if err != nil {
			t.Fatalf("expect not to be failed")
		}
		resolver.Stop()
	}

	{
		resolver, err := New(0, 0)
		if err != nil {
			t.Fatalf("expect not to be failed")
		}
		resolver.Stop()
	}
}

func TestLookup(t *testing.T) {
	cases := []struct {
		name string
	}{
		{"go.mercari.io"},
		{"yandex.ru"},
		{"google.com"},
	}

	resolver := testResolver(t)
	defer resolver.Stop()
	for _, tc := range cases {
		ips, err := resolver.LookupIP(context.Background(), tc.name)
		if err != nil {
			t.Fatalf("err: %s", err)
		}
		if len(ips) == 0 {
			t.Fatalf("got no records")
		}

		for _, ip := range ips {
			if ip.To4() == nil && ip.To16() == nil {
				t.Fatalf("got %v; want an IP address", ip)
			}
		}
	}
}

func TestLookupCache(t *testing.T) {
	want := []net.IP{
		net.IP("35.190.50.136"),
	}

	ctx := context.Background()
	resolver := testResolver(t)
	defer resolver.Stop()
	setLookupUpFn(t, resolver, func(ctx context.Context, host string) ([]net.IP, error) {
		return want, nil
	})

	got, err := resolver.LookupIP(ctx, "gateway.io")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if !reflect.DeepEqual(want, got) {
		t.Fatalf("want %#v, got %#v", want, got)
	}

	got2, ok := resolver.cache["gateway.io"]
	if !ok {
		t.Fatalf("expect cache to be created")
	}

	if !reflect.DeepEqual(want, got2) {
		t.Fatalf("want %#v, got %#v", want, got2)
	}
}

func TestLookupTimeout(t *testing.T) {
	ctx, cancelF := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelF()

	resolver := testResolver(t)
	defer resolver.Stop()
	setLookupUpFn(t, resolver, func(ctx context.Context, host string) ([]net.IP, error) {
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			time.Sleep(200 * time.Millisecond)
		}
	})

	_, err := resolver.LookupIP(ctx, "gateway.io")
	if err == nil {
		t.Fatalf("expect to be failed")
	}
}

func TestRefresh(t *testing.T) {
	want := []net.IP{
		net.IP("4.4.4.4"),
	}

	resolver := testResolver(t)
	defer resolver.Stop()
	setLookupUpFn(t, resolver, func(ctx context.Context, host string) ([]net.IP, error) {
		return want, nil
	})
	resolver.cache = map[string][]net.IP{
		"deeeet.jp": {
			net.IP("1.1.1.1"),
		},
		"deeeet.us": {
			net.IP("2.2.2.2"),
		},
		"deeeet.uk": {
			net.IP("3.3.3.3"),
		},
	}

	// Refresh all IP to same one
	resolver.Refresh()

	// Ensure all cache are refreshed
	for _, got := range resolver.cache {
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("want %#v, got %#v", want, got)
		}
	}
}

func TestRefreshed(t *testing.T) {
	var counter int32

	resolver, err := New(time.Millisecond, testDefaultLookupTimeout)
	defer resolver.Stop()

	setLookupUpFn(t, resolver, func(ctx context.Context, host string) ([]net.IP, error) {
		atomic.AddInt32(&counter, 1)
		return []net.IP{net.IP("127.0.0.1")}, nil
	})

	// add single record to cache to make refresh happen
	resolver.LookupIP(context.Background(), "whatever.com")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	cnt := atomic.LoadInt32(&counter)
	if cnt < 5 {
		t.Fatalf("Not refreshed enough: %d", cnt)
	}
}

func TestFetch(t *testing.T) {
	var mu sync.Mutex
	var returnIPs []net.IP

	ctx := context.Background()
	resolver := testResolver(t)
	defer resolver.Stop()

	setLookupUpFn(t, resolver, func(ctx context.Context, host string) ([]net.IP, error) {
		mu.Lock()
		ips := returnIPs
		mu.Unlock()
		return ips, nil
	})

	want1 := []net.IP{
		net.IP("10.0.0.1"),
	}
	mu.Lock()
	returnIPs = want1
	mu.Unlock()

	got1, err := resolver.Fetch(ctx, "test.com")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if !reflect.DeepEqual(want1, got1) {
		t.Fatalf("want %#v, got %#v", want1, got1)
	}

	want2 := []net.IP{
		net.IP("10.0.0.2"),
	}
	mu.Lock()
	returnIPs = want2
	mu.Unlock()

	got2, err := resolver.Fetch(ctx, "test.com")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// Cache should be used
	if !reflect.DeepEqual(want1, got2) {
		t.Fatalf("want %#v, got %#v", want1, got2)
	}

	// Wait until cache is refreshed
	time.Sleep(2 * time.Second)

	got3, err := resolver.Fetch(ctx, "test.com")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	// Cache should be refreshed
	if !reflect.DeepEqual(want2, got3) {
		t.Fatalf("want %#v, got %#v", want2, got3)
	}
}

type logsWriter struct {
	bytes.Buffer
	mu sync.Mutex
}

func (w *logsWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Buffer.Write(p)
}

func (w *logsWriter) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.Buffer.Len()
}

func TestErrorLog(t *testing.T) {
	testCases := map[string]struct {
		cache     map[string][]net.IP
		expectErr bool
	}{
		"empty cache: no error": {},
		"one item in cache: expect err": {
			cache:     map[string][]net.IP{"ya.ru": {net.IP("127.0.0.1")}},
			expectErr: true,
		},
	}

	for caseName, testCase := range testCases {
		t.Run(caseName, func(t *testing.T) {
			var logs = new(logsWriter)
			logger := slog.New(slog.NewJSONHandler(logs, nil))

			resolver, err := New(time.Millisecond, 0, WithLogger(logger))
			if err != nil {
				t.Fatalf("err: %s", err)
			}
			defer resolver.Stop()

			setLookupUpFn(t, resolver, func(context.Context, string) (res []net.IP, err error) {
				return nil, errors.New("err")
			})

			resolver.cacheMu.Lock()
			resolver.cache = testCase.cache
			resolver.cacheMu.Unlock()

			<-time.After(5 * time.Millisecond)

			if testCase.expectErr {
				if logs.Len() == 0 {
					t.Fatalf("expected error to be logged, none found")
				}
			} else if logs.Len() > 0 {
				t.Fatalf("unexpected logs with empty cache")
			}
		})
	}
}
