package webui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSPAAndAssetServing(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><title>Agent Room</title>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.abc.js"), []byte("export{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer handler.Close()
	tests := []struct {
		path, accept, cache string
		status              int
	}{
		{"/", "text/html", "no-cache", 200},
		{"/tasks/one", "text/html", "no-cache", 200},
		{"/assets/app.abc.js", "*/*", "immutable", 200},
		{"/missing.js", "text/html", "", 404},
		{"/api/v1/unknown", "text/html", "", 404},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.Header.Set("Accept", test.accept)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%s status=%d", test.path, response.Code)
		}
		if test.cache != "" && !strings.Contains(response.Header().Get("Cache-Control"), test.cache) {
			t.Errorf("%s cache=%q", test.path, response.Header().Get("Cache-Control"))
		}
	}
}
