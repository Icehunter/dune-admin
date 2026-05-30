package main

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// memFS is a minimal http.FileSystem backed by an in-memory map for testing.
type memFS map[string]string

func (m memFS) Open(name string) (http.File, error) {
	content, ok := m[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &memFile{content: content, r: strings.NewReader(content)}, nil
}

type memFile struct {
	content string
	r       *strings.Reader
}

func (f *memFile) Read(p []byte) (int, error)                { return f.r.Read(p) }
func (f *memFile) Seek(off int64, whence int) (int64, error) { return f.r.Seek(off, whence) }
func (f *memFile) Close() error                              { return nil }
func (f *memFile) Readdir(_ int) ([]fs.FileInfo, error)      { return nil, nil }
func (f *memFile) Stat() (fs.FileInfo, error)                { return &memFileInfo{size: int64(len(f.content))}, nil }

type memFileInfo struct{ size int64 }

func (fi *memFileInfo) Name() string       { return "file" }
func (fi *memFileInfo) Size() int64        { return fi.size }
func (fi *memFileInfo) Mode() fs.FileMode  { return 0o444 }
func (fi *memFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *memFileInfo) IsDir() bool        { return false }
func (fi *memFileInfo) Sys() any           { return nil }

func TestSpaHandlerFS_NoRedirectLoop(t *testing.T) {
	fsys := memFS{
		"/index.html":     "<html>app</html>",
		"/assets/main.js": "console.log('hi')",
	}

	handler := spaHandlerFS(fsys)

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "root returns index.html (no redirect loop)",
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "<html>app</html>",
		},
		{
			name:       "unknown SPA route falls back to index.html",
			path:       "/players",
			wantStatus: http.StatusOK,
			wantBody:   "<html>app</html>",
		},
		{
			name:       "real asset served directly",
			path:       "/assets/main.js",
			wantStatus: http.StatusOK,
			wantBody:   "console.log('hi')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code == http.StatusMovedPermanently || rr.Code == http.StatusFound {
				t.Fatalf("got redirect %d to %q — causes ERR_TOO_MANY_REDIRECTS in browser",
					rr.Code, rr.Header().Get("Location"))
			}
			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}
			body, _ := io.ReadAll(rr.Body)
			if !strings.Contains(string(body), tt.wantBody) {
				t.Errorf("body %q does not contain %q", string(body), tt.wantBody)
			}
		})
	}
}
