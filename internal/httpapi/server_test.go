package httpapi

import (
	"bytes"
	"encoding/json"
	mimepart "mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalysisHTTP(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "reports"), 0755)
	s := New(nil, dir, strings.Repeat("x", 32))
	h := s.Handler()
	var b bytes.Buffer
	form := mimepart.NewWriter(&b)
	f, _ := form.CreateFormFile("file", "project.json")
	f.Write([]byte(`{"targets":[],"extensions":[]}`))
	form.WriteField("is_sort", "desc")
	form.WriteField("is_high_rank_cate", "top12")
	form.Close()
	req := httptest.NewRequest("POST", "/api/v2/analyze", &b)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	sources, err := os.ReadDir(filepath.Join(dir, "uploads"))
	if err != nil || len(sources) != 1 {
		t.Fatal("accepted source was not retained", err)
	}
	retained, err := os.ReadFile(filepath.Join(dir, "uploads", sources[0].Name()))
	if err != nil || string(retained) != `{"targets":[],"extensions":[]}` {
		t.Fatal("source bytes changed", err)
	}
	wPrivate := httptest.NewRecorder()
	h.ServeHTTP(wPrivate, httptest.NewRequest("GET", "/api/uploads/"+sources[0].Name(), nil))
	if wPrivate.Code != 404 {
		t.Fatal("source upload must not be public")
	}
	var out struct {
		Token string `json:"token"`
	}
	json.Unmarshal(w.Body.Bytes(), &out)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", out.Token, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<svg") {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, url := range []string{"/api/report-img?stamp=../../etc/passwd", "/api/v2/projects-display?n=-1"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", url, nil))
		if w.Code != 400 {
			t.Fatal(url, w.Code)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v2/project-display-review", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
}
func TestURLValidation(t *testing.T) {
	for _, v := range []string{"javascript:alert(1)", "https://", "https://user:pass@host", "/relative"} {
		if validURL(v) {
			t.Fatal(v)
		}
	}
	if !validURL("https://scratch.mit.edu/projects/123") {
		t.Fatal("valid URL rejected")
	}
}

func TestComparisonRetainsBothSources(t *testing.T) {
	dir := t.TempDir()
	handler := New(nil, dir, "").Handler()
	var body bytes.Buffer
	form := mimepart.NewWriter(&body)
	for _, field := range []string{"original", "compared"} {
		file, err := form.CreateFormFile(field, "project.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(`{"targets":[],"extensions":[]}`)); err != nil {
			t.Fatal(err)
		}
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/api/v2/compare", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(dir, "uploads"))
	if err != nil || len(entries) != 2 {
		t.Fatal("both comparison sources must be retained", err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, "uploads", entry.Name()))
		if err != nil || string(data) != `{"targets":[],"extensions":[]}` {
			t.Fatal("retained bytes changed", err)
		}
	}
}
