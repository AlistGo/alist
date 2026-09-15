package quark

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRemoveReliableCancelsInFlightDeleteRequest(t *testing.T) {
	started := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != fileDeletePath {
			http.NotFound(w, r)
			return
		}
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
			http.Error(w, "request context was not canceled", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- d.Remove(ctx, deleteTestObject("fid-inflight"))
	}()

	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("delete request did not start")
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Remove did not return after cancellation")
	}
}

func TestRemoveReliableCancelsInFlightVerifierRequest(t *testing.T) {
	verifyStarted := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == fileDeletePath:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"status":500,"code":500,"message":"inner error, requestId verify-cancel"}`))
		case r.Method == http.MethodGet && r.URL.Path == fileInfoPath:
			verifyStarted <- struct{}{}
			select {
			case <-r.Context().Done():
			case <-time.After(2 * time.Second):
				http.Error(w, "verifier context was not canceled", http.StatusInternalServerError)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- d.Remove(ctx, deleteTestObject("fid-verify-inflight"))
	}()

	select {
	case <-verifyStarted:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("verifier request did not start")
	}

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Remove did not return after verifier cancellation")
	}
}

func TestWaitDeleteRetryReturnsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := waitDeleteRetry(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("waitDeleteRetry blocked for %v, want an immediate return", elapsed)
	}
}

func TestRemoveReliableCancelsDuringRetryBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == fileDeletePath:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"status":500,"code":500,"message":"inner error, requestId backoff-cancel"}`))
		case r.Method == http.MethodGet && r.URL.Path == fileInfoPath:
			// The FID is still present, so the loop enters the backoff wait.
			// Cancel here so the cancellation lands during that wait.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":200,"code":0,"data":{"fid":"fid-backoff","file":true}}`))
			cancel()
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	d := newDeleteTestDriver(srv.URL)
	done := make(chan error, 1)
	go func() {
		done <- d.Remove(ctx, deleteTestObject("fid-backoff"))
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Remove did not return after cancellation during backoff")
	}
}
