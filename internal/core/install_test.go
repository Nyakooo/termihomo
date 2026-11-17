package core

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDownloadKernelArchiveSelectsVerifiedSource(t *testing.T) {
	const payload = "expected archive"
	stalled := make(chan struct{})
	cancelled := make(chan struct{})
	invalid := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stalled":
			w.(http.Flusher).Flush()
			close(stalled)
			<-r.Context().Done()
			close(cancelled)
		case "/invalid":
			io.WriteString(w, "unexpected archive")
			close(invalid)
		case "/valid":
			for _, ready := range []<-chan struct{}{stalled, invalid} {
				select {
				case <-ready:
				case <-r.Context().Done():
					return
				}
			}
			io.WriteString(w, payload)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	dir := t.TempDir()
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
	sources := []string{server.URL + "/stalled", server.URL + "/invalid", server.URL + "/valid"}
	archive, size, err := downloadKernelArchive(ctx, server.Client(), sources, dir, digest)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if size != int64(len(payload)) {
		t.Fatalf("unexpected archive size %d", size)
	}
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("stalled download was not cancelled")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("expected only the verified archive, got %v (%v)", files, err)
	}
}

func TestDownloadKernelArchiveReportsInvalidAndUnavailableSources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/unavailable" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, "unexpected archive")
	}))
	defer server.Close()

	dir := t.TempDir()
	sources := []string{server.URL + "/unavailable", server.URL + "/invalid"}
	_, _, err := downloadKernelArchive(t.Context(), server.Client(), sources, dir, "invalid digest")
	if err == nil || !strings.Contains(err.Error(), "HTTP 503") || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected both source failures, got %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatalf("expected failed downloads to be removed, got %v (%v)", files, err)
	}
}
