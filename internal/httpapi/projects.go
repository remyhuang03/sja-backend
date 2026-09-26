package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/remyhuang03/sja-backend/internal/store"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && len(raw) <= 2048
}
func validateMeta(m *store.Meta) error {
	m.ProjectName = strings.TrimSpace(m.ProjectName)
	m.AuthorName = strings.TrimSpace(m.AuthorName)
	m.Brief = strings.TrimSpace(m.Brief)
	if m.ProjectName == "" || utf8.RuneCountInString(m.ProjectName) > 80 || m.AuthorName == "" || utf8.RuneCountInString(m.AuthorName) > 80 {
		return errors.New("作品与作者名称须为 1–80 字")
	}
	if m.Brief == "" || utf8.RuneCountInString(m.Brief) > 20 {
		return errors.New("作品简介须为 1–20 字")
	}
	if !validURL(m.AuthorLink) {
		return errors.New("作者主页 URL 无效")
	}
	if len(m.Links) < 1 || len(m.Links) > 10 {
		return errors.New("请提供 1–10 个作品链接")
	}
	defaults := 0
	for _, l := range m.Links {
		if !validURL(l.URL) || len(l.Platform) > 80 {
			return errors.New("作品链接无效")
		}
		if l.Default {
			defaults++
		}
	}
	if defaults != 1 {
		return errors.New("必须指定一个默认作品链接")
	}
	return nil
}
func saveImage(r *http.Request, key, path string, limit int64) error {
	f, h, err := r.FormFile(key)
	if err != nil {
		return errors.New("缺少图片 " + key)
	}
	defer f.Close()
	if h.Size > limit {
		return errors.New("图片超过大小限制")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return errors.New("读取图片失败或图片过大")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "jpeg" && format != "png" && format != "webp") {
		return errors.New("仅支持有效的 JPEG、PNG、WebP 图片")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 12000000 {
		return errors.New("图片尺寸过大，最长边须不超过 4096 像素")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return errors.New("无法解码图片")
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	err = png.Encode(out, normalizeShowcaseImage(img, key))
	closeErr := out.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func (s *Server) apply(w http.ResponseWriter, r *http.Request) {
	// Image decoding also shares the bounded worker budget.
	select {
	case s.jobs <- struct{}{}:
		defer func() { <-s.jobs }()
	default:
		fail(w, 429, "当前提交较多，请稍后重试")
		return
	}
	if !multipart(w, r, 8<<20) {
		return
	}
	defer r.MultipartForm.RemoveAll()
	var meta store.Meta
	if json.Unmarshal([]byte(r.FormValue("meta")), &meta) != nil {
		fail(w, 400, "meta 必须是有效 JSON")
		return
	}
	if err := validateMeta(&meta); err != nil {
		fail(w, 400, err.Error())
		return
	}
	id := randomID()
	dir := filepath.Join(s.DataDir, "media", id)
	if err := os.Mkdir(dir, 0755); err != nil {
		s.internal(w, err)
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(dir)
		}
	}()
	for _, spec := range []struct {
		key   string
		limit int64
	}{{"cover", 5 << 20}, {"avatar", 2 << 20}} {
		if err := saveImage(r, spec.key, filepath.Join(dir, spec.key+".png"), spec.limit); err != nil {
			fail(w, 400, err.Error())
			return
		}
	}
	a := store.Application{ID: id, Meta: meta, CoverPath: "/api/media/" + id + "/cover.png", AvatarPath: "/api/media/" + id + "/avatar.png"}
	if err := s.Store.Create(r.Context(), a); err != nil {
		s.internal(w, err)
		return
	}
	committed = true
	writeJSON(w, 201, map[string]any{"status": "ok", "message": "申请提交成功，等待审核", "data": map[string]string{"application_id": id}})
}
func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	id, name := r.PathValue("id"), r.PathValue("file")
	if !uuidName.MatchString(id) || (name != "cover.png" && name != "avatar.png" && name != "favicon.png") {
		fail(w, 404, "图片不存在")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	http.ServeFile(w, r, filepath.Join(s.DataDir, "media", id, name))
}
func (s *Server) review(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&input) != nil || !uuidName.MatchString(input.ID) || (input.Status != "approved" && input.Status != "rejected") || utf8.RuneCountInString(input.Notes) > 2000 {
		fail(w, 400, "审核参数无效")
		return
	}
	if input.Status == "rejected" && strings.TrimSpace(input.Notes) == "" {
		fail(w, 400, "请填写拒绝原因")
		return
	}
	err := s.Store.Review(r.Context(), input.ID, input.Status, input.Notes)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "申请不存在")
		return
	}
	if errors.Is(err, store.ErrConflict) {
		fail(w, 409, "申请已审核，请刷新列表")
		return
	}
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"success": true, "message": "审核完成"})
}

// Normalize every accepted upload, including API clients that bypass the browser cropper.
func normalizeShowcaseImage(img image.Image, key string) image.Image {
	width, height := 256, 256
	if key == "cover" {
		width, height = 1200, 900
	}
	bounds := img.Bounds()
	sourceWidth, sourceHeight := bounds.Dx(), bounds.Dy()
	cropWidth, cropHeight := sourceWidth, sourceHeight
	if sourceWidth*height > sourceHeight*width {
		cropWidth = max(1, sourceHeight*width/height)
	} else {
		cropHeight = max(1, sourceWidth*height/width)
	}
	x := bounds.Min.X + (sourceWidth-cropWidth)/2
	y := bounds.Min.Y + (sourceHeight-cropHeight)/2
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.CatmullRom.Scale(out, out.Bounds(), img, image.Rect(x, y, x+cropWidth, y+cropHeight), draw.Src, nil)
	return out
}
