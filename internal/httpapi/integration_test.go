package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	mimepart "mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/remyhuang03/sja-backend/internal/store"
)

func TestApplicationReviewFlow(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run API integration tests")
	}
	ctx := context.Background()
	adminDB, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer adminDB.Pool.Close()
	if _, err = adminDB.Pool.Exec(ctx, "CREATE SCHEMA sja_http_integration"); err != nil {
		t.Fatal(err)
	}
	defer adminDB.Pool.Exec(ctx, "DROP SCHEMA sja_http_integration CASCADE")
	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	db, err := store.Open(ctx, url+sep+"search_path=sja_http_integration")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Pool.Close()
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "media"), 0755)
	token := strings.Repeat("t", 32)
	h := New(db, dir, token).Handler()
	var body bytes.Buffer
	form := mimepart.NewWriter(&body)
	form.WriteField("meta", `{"project_name":"测试作品","author_name":"测试作者","author_link":"https://example.com","brief":"流程验证","links":[{"platform":"test","url":"https://example.com/project","is_default":true}]}`)
	for _, key := range []string{"cover", "avatar"} {
		f, _ := form.CreateFormFile(key, key+".jpg")
		png.Encode(f, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	}
	form.Close()
	req := httptest.NewRequest("POST", "/api/v2/project-display-apply", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var response struct {
		Data struct {
			ID string `json:"application_id"`
		} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	review := func() int {
		req := httptest.NewRequest("POST", "/api/v2/project-display-review", strings.NewReader(`{"id":"`+response.Data.ID+`","status":"approved","notes":"ok"}`))
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}
	if code := review(); code != 200 {
		t.Fatal("review", code)
	}
	if code := review(); code != 409 {
		t.Fatal("duplicate review", code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/v2/projects-display?n=5", nil))
	var projects []store.Project
	json.Unmarshal(w.Body.Bytes(), &projects)
	if w.Code != 200 || len(projects) != 1 {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", projects[0].CoverPath, nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatal("image", w.Code, w.Header())
	}
}
