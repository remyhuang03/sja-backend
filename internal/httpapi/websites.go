package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/remyhuang03/sja-backend/internal/favicon"
	"github.com/remyhuang03/sja-backend/internal/store"
)

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(&struct{}{}) != io.EOF {
		fail(w, 400, "提交内容无效")
		return false
	}
	return true
}
func (s *Server) websiteApply(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		Category    string `json:"category"`
		Description string `json:"description"`
		Consent     bool   `json:"consent"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	categories := map[string]bool{"communities": true, "tools": true, "developers": true, "assets": true, "other": true}
	if !input.Consent || utf8.RuneCountInString(input.Name) < 1 || utf8.RuneCountInString(input.Name) > 80 || utf8.RuneCountInString(input.Description) > 300 || !categories[input.Category] {
		fail(w, 400, "请填写网站名称、分类并确认收录要求")
		return
	}
	normalized, err := favicon.NormalizeURL(input.URL)
	if err != nil {
		fail(w, 400, "请填写有效的公开网站网址")
		return
	}
	item := store.Website{ID: randomID(), Name: input.Name, URL: normalized, Category: input.Category, Description: input.Description}
	if err = s.Store.CreateWebsite(r.Context(), item); err != nil {
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && pgerr.Code == "23505" {
			fail(w, 409, "该网站已收录或正在审核")
			return
		}
		s.internal(w, err)
		return
	}
	writeJSON(w, 201, map[string]string{"message": "网站提交成功，审核通过后将公开显示", "id": item.ID})
}
func (s *Server) websites(w http.ResponseWriter, r *http.Request) {
	items, err := s.Store.Websites(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) websiteSubmissions(w http.ResponseWriter, r *http.Request) {
	limit, ok := integer(r, "limit", 20, 1, 100)
	offset, ok2 := integer(r, "offset", 0, 0, 1000000)
	if !ok || !ok2 {
		fail(w, 400, "分页参数无效")
		return
	}
	items, err := s.Store.WebsiteSubmissions(r.Context(), limit, offset)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) websiteReview(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if !readJSON(w, r, &input) {
		return
	}
	if !uuidName.MatchString(input.ID) || (input.Status != "approved" && input.Status != "rejected") || utf8.RuneCountInString(input.Notes) > 2000 {
		fail(w, 400, "审核参数无效")
		return
	}
	if input.Status == "rejected" && strings.TrimSpace(input.Notes) == "" {
		fail(w, 400, "请填写拒绝原因")
		return
	}
	item, err := s.Store.Website(r.Context(), input.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "申请不存在")
		return
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	if item.Status != "pending" {
		fail(w, 409, "申请已审核，请刷新列表")
		return
	}
	icon := ""
	if input.Status == "approved" {
		if data := s.fetchIcon(r.Context(), item.URL); len(data) > 0 {
			// Use a unique media directory so simultaneous review attempts cannot overwrite each other.
			id := randomID()
			dir := filepath.Join(s.DataDir, "media", id)
			if err = os.MkdirAll(dir, 0755); err == nil {
				err = os.WriteFile(filepath.Join(dir, "favicon.png"), data, 0644)
			}
			if err != nil {
				s.internal(w, err)
				return
			}
			icon = "/api/media/" + id + "/favicon.png"
		}
	}
	err = s.Store.ReviewWebsite(r.Context(), input.ID, input.Status, input.Notes, icon)
	if err != nil && icon != "" {
		_ = os.Remove(filepath.Join(s.DataDir, strings.TrimPrefix(icon, "/api/")))
	}
	if errors.Is(err, store.ErrConflict) {
		fail(w, 409, "申请已审核，请刷新列表")
		return
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "审核完成"})
}
