package localfile

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func get(target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestHandler(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	txt := filepath.Join(dir, "secret.txt")
	os.WriteFile(img, []byte("PNGDATA"), 0o644)
	os.WriteFile(txt, []byte("secret"), 0o644)

	if rec := get(URL(img)); rec.Code != 200 || rec.Body.String() != "PNGDATA" || rec.Header().Get("Content-Type") != "image/png" {
		t.Errorf("image: %d %q %q", rec.Code, rec.Body, rec.Header().Get("Content-Type"))
	}
	if rec := get(URL(txt)); rec.Code != http.StatusForbidden {
		t.Errorf("non-image served: %d", rec.Code)
	}
	if rec := get(URL("relative/a.png")); rec.Code != http.StatusForbidden {
		t.Errorf("relative path served: %d", rec.Code)
	}
	if rec := get("/other"); rec.Code != http.StatusNotFound {
		t.Errorf("other path: %d", rec.Code)
	}
}
