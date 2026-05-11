package files

import (
	"bytes"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chen-zhanjie/webhook-router/internal/config"
)

func TestSaveMultipartServeAndExpire(t *testing.T) {
	dir := t.TempDir()
	m := New(config.FileConfig{
		StorageDir: dir,
		TTL:        config.Duration{Duration: 20 * time.Millisecond},
		MaxBytes:   1024,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := m.EnsureStorage(); err != nil {
		t.Fatalf("EnsureStorage() error = %v", err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "hello world.txt")
	if err != nil {
		t.Fatalf("CreateFormFile() error = %v", err)
	}
	if _, err := part.Write([]byte("hello")); err != nil {
		t.Fatalf("write part error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	req := httptest.NewRequest("POST", "/apps/app/files", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	result, err := m.SaveMultipart(httptest.NewRecorder(), req)
	if err != nil {
		t.Fatalf("SaveMultipart() error = %v", err)
	}
	if result.Path == "" || !strings.Contains(result.Path, "hello%20world.txt") {
		t.Fatalf("unexpected returned path: %q", result.Path)
	}

	parts := strings.Split(strings.TrimPrefix(result.Path, "/files/"), "/")
	if len(parts) != 2 {
		t.Fatalf("unexpected path parts: %#v", parts)
	}
	filename, err := url.PathUnescape(parts[1])
	if err != nil {
		t.Fatalf("PathUnescape() error = %v", err)
	}

	res := httptest.NewRecorder()
	m.Serve(res, httptest.NewRequest("GET", result.Path, nil), parts[0], filename)
	if res.Code != 200 {
		t.Fatalf("Serve() code = %d, want 200", res.Code)
	}
	if got := res.Body.String(); got != "hello" {
		t.Fatalf("Serve() body = %q, want hello", got)
	}

	time.Sleep(30 * time.Millisecond)
	res = httptest.NewRecorder()
	m.Serve(res, httptest.NewRequest("GET", result.Path, nil), parts[0], filename)
	if res.Code != 404 {
		t.Fatalf("expired Serve() code = %d, want 404", res.Code)
	}
	if entries, err := filepath.Glob(filepath.Join(dir, "*")); err != nil || len(entries) != 0 {
		t.Fatalf("expired file directory was not removed, entries=%v err=%v", entries, err)
	}
}
