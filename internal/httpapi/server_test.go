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
