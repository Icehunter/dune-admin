package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// TestServeWithGracefulShutdown_StopsOnCtxCancel locks in the contract that a
// cancelled context makes serveWithGracefulShutdown return (via srv.Shutdown)
// instead of blocking forever in ListenAndServe.
func TestServeWithGracefulShutdown_StopsOnCtxCancel(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- serveWithGracefulShutdown(ctx, srv, time.Second) }()
	cancel() // ctx.Done drives Shutdown whether or not ListenAndServe has bound yet
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("graceful shutdown returned %v, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("serveWithGracefulShutdown did not return after ctx cancel")
	}
}

// TestServeWithGracefulShutdown_ReturnsListenError locks in that a failed
// ListenAndServe (here: an out-of-range port) is surfaced as an error.
func TestServeWithGracefulShutdown_ReturnsListenError(t *testing.T) {
	srv := &http.Server{Addr: "127.0.0.1:99999999", Handler: http.NewServeMux()}
	err := serveWithGracefulShutdown(context.Background(), srv, time.Second)
	if err == nil || errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("want a real listen error, got %v", err)
	}
}

// TestPreferServeErr: a real ListenAndServe failure wins over the shutdown
// outcome (so a bind error racing with ctx cancellation is not swallowed); an
// ErrServerClosed serve result falls through to the mapped shutdown result.
func TestPreferServeErr(t *testing.T) {
	bind := errors.New("listen tcp: address already in use")
	if got := preferServeErr(bind, nil); !errors.Is(got, bind) {
		t.Errorf("real serve error must win: got %v, want %v", got, bind)
	}
	// Clean serve stop + clean shutdown → nil.
	if got := preferServeErr(http.ErrServerClosed, nil); got != nil {
		t.Errorf("clean stop: got %v, want nil", got)
	}
	// Clean serve stop + grace timeout → nil (bounded shutdown, not a failure).
	if got := preferServeErr(http.ErrServerClosed, context.DeadlineExceeded); got != nil {
		t.Errorf("grace timeout: got %v, want nil", got)
	}
	// Clean serve stop + a non-deadline shutdown error → that error propagates.
	listenerErr := errors.New("listener close failed")
	if got := preferServeErr(http.ErrServerClosed, listenerErr); !errors.Is(got, listenerErr) {
		t.Errorf("shutdown error: got %v, want %v", got, listenerErr)
	}
}

// TestIgnoreServerClosed: ErrServerClosed is a clean stop; other errors pass through.
func TestIgnoreServerClosed(t *testing.T) {
	if got := ignoreServerClosed(http.ErrServerClosed); got != nil {
		t.Errorf("ErrServerClosed: got %v, want nil", got)
	}
	if got := ignoreServerClosed(nil); got != nil {
		t.Errorf("nil: got %v, want nil", got)
	}
	boom := errors.New("listen: address in use")
	if got := ignoreServerClosed(boom); !errors.Is(got, boom) {
		t.Errorf("other error: got %v, want %v", got, boom)
	}
}

// TestDrainResult: a grace-deadline timeout is an intentional bounded shutdown
// (force-close) and must NOT become a non-zero exit; other errors propagate.
func TestDrainResult(t *testing.T) {
	if got := drainResult(context.DeadlineExceeded); got != nil {
		t.Errorf("DeadlineExceeded: got %v, want nil (force-close is not a failure)", got)
	}
	if got := drainResult(nil); got != nil {
		t.Errorf("nil: got %v, want nil", got)
	}
	boom := errors.New("listener close failed")
	if got := drainResult(boom); !errors.Is(got, boom) {
		t.Errorf("other error: got %v, want %v", got, boom)
	}
}
