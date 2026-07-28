package webui

import (
	"errors"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
)

type Handler struct{ root *os.Root }

func Open(directory string) (*Handler, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	info, err := fs.Stat(root.FS(), "index.html")
	if err != nil || !info.Mode().IsRegular() {
		_ = root.Close()
		return nil, errors.New("web directory must contain a regular index.html")
	}
	return &Handler{root: root}, nil
}

func (h *Handler) Close() error { return h.root.Close() }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "." || name == "" {
		name = "index.html"
	}
	if !fs.ValidPath(name) || strings.HasPrefix(name, "api/") {
		http.NotFound(w, r)
		return
	}
	if h.serveFile(w, r, name, strings.HasPrefix(name, "assets/")) {
		return
	}
	if path.Ext(name) != "" || !strings.Contains(r.Header.Get("Accept"), "text/html") {
		http.NotFound(w, r)
		return
	}
	if !h.serveFile(w, r, "index.html", false) {
		http.NotFound(w, r)
	}
}

func (h *Handler) serveFile(w http.ResponseWriter, r *http.Request, name string, immutable bool) bool {
	file, err := h.root.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if immutable {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if name == "index.html" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, info.ModTime(), file)
	return true
}
