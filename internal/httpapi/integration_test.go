package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
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
	api := New(db, dir, token)
	iconFetches := 0
	api.fetchIcon = func(context.Context, string) []byte {
		iconFetches++
		var b bytes.Buffer
		png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 32, 32)))
		return b.Bytes()
	}
	h := api.Handler()
	var body bytes.Buffer
	form := mimepart.NewWriter(&body)
	form.WriteField("meta", `{"project_name":"测试作品","author_name":"测试作者","author_link":"https://example.com","brief":"流程验证","links":[{"platform":"test","url":"https://example.com/project","is_default":true},{"platform":"second","url":"https://example.org/second"}]}`)
	for _, key := range []string{"cover", "avatar"} {
		f, _ := form.CreateFormFile(key, key+".jpg")
		if key == "cover" {
			jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 8, 4)), nil)
		} else {
			png.Encode(f, image.NewRGBA(image.Rect(0, 0, 4, 4)))
		}
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
	if w.Code != 200 || len(projects) != 1 || len(projects[0].Links) != 2 || !projects[0].Links[0].Default {
		t.Fatal(w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", projects[0].CoverPath, nil))
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatal("image", w.Code, w.Header())
	}
	for _, spec := range []struct {
		path          string
		width, height int
	}{{projects[0].CoverPath, 1200, 900}, {projects[0].AvatarPath, 256, 256}} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest("GET", spec.path, nil))
		cfg, format, err := image.DecodeConfig(bytes.NewReader(recorder.Body.Bytes()))
		if err != nil || format != "png" || cfg.Width != spec.width || cfg.Height != spec.height {
			t.Fatalf("normalized image: %+v %s %v", cfg, format, err)
		}
	}
	call := func(method, path, body string, authorized bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if authorized {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, req)
		return recorder
	}
	payload := `{"name":"Scratch test","url":"https://example.org","category":"tools","description":"A test resource","consent":true}`
	submitted := call("POST", "/api/v2/website-apply", payload, false)
	if submitted.Code != 201 {
		t.Fatal(submitted.Code, submitted.Body.String())
	}
	var websiteResponse map[string]string
	json.Unmarshal(submitted.Body.Bytes(), &websiteResponse)
	id := websiteResponse["id"]
	if got := call("POST", "/api/v2/website-apply", payload, false); got.Code != 409 {
		t.Fatal("duplicate website", got.Code)
	}
	if got := call("GET", "/api/v2/websites", "", false); got.Code != 200 || strings.TrimSpace(got.Body.String()) != "[]" {
		t.Fatal("pending site leaked", got.Body.String())
	}
	reviewBody := `{"id":"` + id + `","status":"approved","notes":"verified"}`
	if got := call("POST", "/api/v2/website-review", reviewBody, false); got.Code != 401 {
		t.Fatal("unauthorized review", got.Code)
	}
	if iconFetches != 0 {
		t.Fatal("icon fetched before authorized approval")
	}
	if got := call("POST", "/api/v2/website-review", reviewBody, true); got.Code != 200 {
		t.Fatal("website approval", got.Code, got.Body.String())
	}
	if got := call("POST", "/api/v2/website-review", reviewBody, true); got.Code != 409 {
		t.Fatal("repeated website approval", got.Code)
	}
	if iconFetches != 1 {
		t.Fatal("unexpected icon fetches", iconFetches)
	}
	published := call("GET", "/api/v2/websites", "", false)
	var websites []store.Website
	json.Unmarshal(published.Body.Bytes(), &websites)
	if len(websites) != 1 || websites[0].Name != "Scratch test" || websites[0].Category != "tools" || websites[0].IconPath == "" {
		t.Fatal("website publication", published.Body.String())
	}
	if strings.Contains(published.Body.String(), "reviewer_notes") {
		t.Fatal("private notes leaked")
	}
	icon := call("GET", websites[0].IconPath, "", false)
	if icon.Code != 200 || icon.Header().Get("Content-Type") != "image/png" {
		t.Fatal("website icon", icon.Code)
	}
	pending := call("GET", "/api/v2/website-review?limit=20&offset=0", "", true)
	if pending.Code != 200 || !strings.Contains(pending.Body.String(), "approved") {
		t.Fatal("website queue", pending.Body.String())
	}
	payload = strings.Replace(payload, "example.org", "example.net", 1)
	submitted = call("POST", "/api/v2/website-apply", payload, false)
	json.Unmarshal(submitted.Body.Bytes(), &websiteResponse)
	rejectBody := `{"id":"` + websiteResponse["id"] + `","status":"rejected","notes":""}`
	if got := call("POST", "/api/v2/website-review", rejectBody, true); got.Code != 400 {
		t.Fatal("rejection requires notes")
	}
	rejectBody = strings.Replace(rejectBody, `"notes":""`, `"notes":"Not relevant"`, 1)
	if got := call("POST", "/api/v2/website-review", rejectBody, true); got.Code != 200 {
		t.Fatal("rejection", got.Body.String())
	}
	published = call("GET", "/api/v2/websites", "", false)
	json.Unmarshal(published.Body.Bytes(), &websites)
	if len(websites) != 1 {
		t.Fatal("rejected site leaked")
	}

}
