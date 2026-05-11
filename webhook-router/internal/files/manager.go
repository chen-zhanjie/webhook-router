package files

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/chen-zhanjie/webhook-router/internal/config"
)

const formFileField = "file"

type Manager struct {
	storageDir string
	ttl        time.Duration
	maxBytes   int64
	log        *slog.Logger
}

type UploadResult struct {
	OK        bool      `json:"ok"`
	Path      string    `json:"path"`
	ExpiresAt time.Time `json:"expires_at"`
	Size      int64     `json:"size"`
	Filename  string    `json:"filename"`
}

type metadata struct {
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Size        int64     `json:"size"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func New(cfg config.FileConfig, log *slog.Logger) *Manager {
	return &Manager{storageDir: cfg.StorageDir, ttl: cfg.TTL.Duration, maxBytes: cfg.MaxBytes, log: log}
}

func (m *Manager) EnsureStorage() error {
	return os.MkdirAll(m.storageDir, 0o750)
}

func (m *Manager) StartCleanup(ctxDone <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	go func() {
		defer ticker.Stop()
		m.cleanupExpired()
		for {
			select {
			case <-ctxDone:
				return
			case <-ticker.C:
				m.cleanupExpired()
			}
		}
	}()
}

func (m *Manager) SaveMultipart(w http.ResponseWriter, r *http.Request) (*UploadResult, error) {
	r.Body = http.MaxBytesReader(w, r.Body, m.maxBytes)
	if err := r.ParseMultipartForm(m.maxBytes); err != nil {
		return nil, fmt.Errorf("parse multipart form: %w", err)
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile(formFileField)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	filename := cleanFilename(header.Filename)
	id, err := newFileID()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(m.storageDir, id)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}

	path := filepath.Join(dir, filename)
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return nil, err
	}
	size, copyErr := io.Copy(out, file)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.RemoveAll(dir)
		return nil, copyErr
	}
	if closeErr != nil {
		_ = os.RemoveAll(dir)
		return nil, closeErr
	}

	now := time.Now().UTC()
	meta := metadata{
		Filename:    filename,
		ContentType: contentType(header.Header.Get("Content-Type"), filename),
		Size:        size,
		CreatedAt:   now,
		ExpiresAt:   now.Add(m.ttl),
	}
	if err := writeMetadata(dir, meta); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	return &UploadResult{OK: true, Path: "/files/" + id + "/" + url.PathEscape(filename), ExpiresAt: meta.ExpiresAt, Size: size, Filename: filename}, nil
}

func (m *Manager) Serve(w http.ResponseWriter, r *http.Request, id, filename string) {
	filename = cleanFilename(filename)
	if !validFileID(id) || filename == "" {
		http.NotFound(w, r)
		return
	}
	dir := filepath.Join(m.storageDir, id)
	meta, err := readMetadata(dir)
	if err != nil || meta.Filename != filename {
		http.NotFound(w, r)
		return
	}
	if time.Now().UTC().After(meta.ExpiresAt) {
		_ = os.RemoveAll(dir)
		http.NotFound(w, r)
		return
	}
	path := filepath.Join(dir, meta.Filename)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", meta.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": meta.Filename}))
	http.ServeFile(w, r, path)
}

func (m *Manager) cleanupExpired() {
	entries, err := os.ReadDir(m.storageDir)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			m.log.Warn("scan temp files failed", "error", err)
		}
		return
	}
	now := time.Now().UTC()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(m.storageDir, entry.Name())
		meta, err := readMetadata(dir)
		if err != nil || now.After(meta.ExpiresAt) {
			if err := os.RemoveAll(dir); err != nil {
				m.log.Warn("remove expired temp file failed", "dir", dir, "error", err)
			}
		}
	}
}

func writeMetadata(dir string, meta metadata) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), raw, 0o640)
}

func readMetadata(dir string) (metadata, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		return metadata{}, err
	}
	var meta metadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return metadata{}, err
	}
	return meta, nil
}

func cleanFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "." || name == "" {
		return "file"
	}
	return name
}

func validFileID(id string) bool {
	if len(id) != 26 {
		return false
	}
	for _, r := range id {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

func contentType(uploaded, filename string) string {
	if uploaded != "" {
		return uploaded
	}
	if guessed := mime.TypeByExtension(filepath.Ext(filename)); guessed != "" {
		return guessed
	}
	return "application/octet-stream"
}

func newFileID() (string, error) {
	entropy := ulid.Monotonic(rand.Reader, 0)
	id, err := ulid.New(ulid.Timestamp(time.Now()), entropy)
	if err != nil {
		return "", err
	}
	return strings.ToLower(id.String()), nil
}
