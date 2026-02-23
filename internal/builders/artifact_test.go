package builders

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownloadArtifact_Success(t *testing.T) {
	payload := []byte("hello artifact")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	data, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/test.whl", downloadArtifactOptions{
		MaxSize: 1024,
	})
	if err != nil {
		t.Fatalf("downloadArtifact() error = %v", err)
	}
	if string(data) != "hello artifact" {
		t.Errorf("data = %q, want %q", string(data), "hello artifact")
	}
}

func TestDownloadArtifact_RejectsHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/test.whl", downloadArtifactOptions{
		MaxSize: 1024,
	})
	if err == nil {
		t.Fatal("expected error for HTTP URL")
	}
	if !strings.Contains(err.Error(), "HTTPS") {
		t.Errorf("error = %v, want mention of HTTPS", err)
	}
}

func TestDownloadArtifact_ExceedsSizeLimit(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write 100 bytes
		_, _ = w.Write(make([]byte, 100))
	}))
	defer server.Close()

	_, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/big.whl", downloadArtifactOptions{
		MaxSize: 50,
	})
	if err == nil {
		t.Fatal("expected error for oversized artifact")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("error = %v, want size limit message", err)
	}
}

func TestDownloadArtifact_SHA256Match(t *testing.T) {
	payload := []byte("verifiable content")
	hash := sha256.Sum256(payload)
	expected := hex.EncodeToString(hash[:])

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	data, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/test.whl", downloadArtifactOptions{
		MaxSize:        1024,
		ExpectedSHA256: expected,
	})
	if err != nil {
		t.Fatalf("downloadArtifact() error = %v", err)
	}
	if string(data) != "verifiable content" {
		t.Errorf("data = %q", string(data))
	}
}

func TestDownloadArtifact_SHA256Mismatch(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("unexpected content"))
	}))
	defer server.Close()

	_, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/test.whl", downloadArtifactOptions{
		MaxSize:        1024,
		ExpectedSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	})
	if err == nil {
		t.Fatal("expected error for SHA256 mismatch")
	}
	if !strings.Contains(err.Error(), "SHA256 mismatch") {
		t.Errorf("error = %v, want SHA256 mismatch message", err)
	}
}

func TestDownloadArtifact_ContentTypeVerification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not a wheel</html>"))
	}))
	defer server.Close()

	_, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/test.whl", downloadArtifactOptions{
		MaxSize:              1024,
		ExpectedContentTypes: []string{"application/zip", "application/octet-stream"},
	})
	if err == nil {
		t.Fatal("expected error for wrong content type")
	}
	if !strings.Contains(err.Error(), "content-type") {
		t.Errorf("error = %v, want content-type message", err)
	}
}

func TestDownloadArtifact_ContentTypeAccepted(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte("data"))
	}))
	defer server.Close()

	_, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/test.whl", downloadArtifactOptions{
		MaxSize:              1024,
		ExpectedContentTypes: []string{"application/zip", "application/octet-stream"},
	})
	if err != nil {
		t.Fatalf("downloadArtifact() error = %v", err)
	}
}

func TestDownloadArtifact_NonOKStatus(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/missing.whl", downloadArtifactOptions{
		MaxSize: 1024,
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "status 404") {
		t.Errorf("error = %v, want status 404 message", err)
	}
}

func TestDownloadArtifact_UserAgentHeader(t *testing.T) {
	var receivedUA string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	_, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/test.whl", downloadArtifactOptions{
		MaxSize: 1024,
	})
	if err != nil {
		t.Fatalf("downloadArtifact() error = %v", err)
	}
	if !strings.Contains(receivedUA, "tsuku/1.0") {
		t.Errorf("User-Agent = %q, want to contain %q", receivedUA, "tsuku/1.0")
	}
}

func TestDownloadArtifact_ExactSizeLimitAllowed(t *testing.T) {
	// A payload that is exactly at the size limit should succeed.
	payload := make([]byte, 50)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	data, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/exact.whl", downloadArtifactOptions{
		MaxSize: 50,
	})
	if err != nil {
		t.Fatalf("downloadArtifact() error = %v", err)
	}
	if len(data) != 50 {
		t.Errorf("len(data) = %d, want 50", len(data))
	}
}

func TestDownloadArtifact_ContextCanceled(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("data"))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := downloadArtifact(ctx, server.Client(), server.URL+"/test.whl", downloadArtifactOptions{
		MaxSize: 1024,
	})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

func TestDownloadArtifact_NoContentTypeCheck(t *testing.T) {
	// When ExpectedContentTypes is empty, any content type is accepted.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("data"))
	}))
	defer server.Close()

	_, err := downloadArtifact(context.Background(), server.Client(), server.URL+"/test.whl", downloadArtifactOptions{
		MaxSize: 1024,
	})
	if err != nil {
		t.Fatalf("downloadArtifact() error = %v", err)
	}
}
