package dnscache

import (
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"time"
)

func ExampleDialFunc() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	resolver, _ := New(3*time.Second, 5*time.Second, logger)

	// You can create a HTTP client which selects an IP from dnscache
	// randomly and dials it.
	rand.Seed(time.Now().UTC().UnixNano()) // You MUST run in once in your application
	client := http.Client{
		Transport: &http.Transport{
			DialContext: DialFunc(resolver, nil),
		},
	}

	// Do what you want.
	_ = client
}
