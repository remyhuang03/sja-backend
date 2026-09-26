package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/remyhuang03/sja-backend/internal/analyzer"
	"github.com/remyhuang03/sja-backend/internal/localize"
	"github.com/remyhuang03/sja-backend/internal/store"
)

type Server struct {
	Store               *store.Store
	DataDir, AdminToken string
	jobs                chan struct{}
}

func New(db *store.Store, dir, token string) *Server {
	return &Server{Store: db, DataDir: dir, AdminToken: token, jobs: make(chan struct{}, 2)}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if s.Store == nil || s.Store.Pool.Ping(ctx) != nil {
			fail(w, 503, "数据库尚未就绪")
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/v2/analyze", s.compute(s.analyze))
	mux.HandleFunc("POST /api/v2/compare", s.compute(s.compare))
	mux.HandleFunc("GET /api/report-img", s.report)
	mux.HandleFunc("POST /api/v2/project-display-apply", s.apply)
	mux.HandleFunc("GET /api/v2/projects-display", s.projects)
	mux.HandleFunc("GET /api/v2/project-display-review", s.admin(s.applications))
	mux.HandleFunc("POST /api/v2/project-display-review", s.admin(s.review))
	mux.HandleFunc("GET /api/media/{id}/{file}", s.media)
	mux.HandleFunc("POST /api/analyze", func(w http.ResponseWriter, r *http.Request) { fail(w, 410, "请使用 /api/v2/analyze") })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w = &localizedWriter{ResponseWriter: w, locale: localize.Locale(r)}
		started := time.Now()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		defer func() {
			if e := recover(); e != nil {
				slog.Error("request panic", "error", e)
				fail(w, 500, "服务器内部错误")
			}
			slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
		}()
		mux.ServeHTTP(w, r)
	})
}

type localizedWriter struct {
	http.ResponseWriter
	locale string
}

func (w *localizedWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func writeJSON(w http.ResponseWriter, status int, v any) {
	if lw, ok := w.(*localizedWriter); ok {
		// Translate only protocol messages, never user-submitted content.
		switch data := v.(type) {
		case map[string]string:
			for _, key := range []string{"message", "msg"} {
				if message, exists := data[key]; exists {
					data[key] = localize.Text(lw.locale, message)
				}
			}
		case map[string]any:
			if message, ok := data["message"].(string); ok {
				data["message"] = localize.Text(lw.locale, message)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"status": "error", "message": msg, "msg": msg})
}
func (s *Server) internal(w http.ResponseWriter, err error) {
	slog.Error("operation failed", "error", err)
	fail(w, 500, "操作失败，请稍后重试")
}
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.AdminToken == "" {
			fail(w, 503, "审核功能未配置")
			return
		}
		expected := sha256.Sum256([]byte(s.AdminToken))
		actual := sha256.Sum256([]byte(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 {
			fail(w, 401, "审核密钥无效")
			return
		}
		next(w, r)
	}
}
func (s *Server) compute(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.jobs <- struct{}{}:
			defer func() { <-s.jobs }()
			next(w, r)
		default:
			w.Header().Set("Retry-After", "3")
			fail(w, 429, "当前分析任务较多，请稍后重试")
		}
	}
}
func multipart(w http.ResponseWriter, r *http.Request, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := r.ParseMultipartForm(2 << 20); err != nil {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			fail(w, 413, "上传文件超过大小限制")
		} else {
			fail(w, 400, "无法解析上传表单")
		}
		return false
	}
	return true
}
func upload(r *http.Request, key string) ([]byte, string, error) {
	f, h, err := r.FormFile(key)
	if err != nil {
		return nil, "", fmt.Errorf("缺少文件 %s", key)
	}
	defer f.Close()
	if h.Size > analyzer.MaxFileSize {
		return nil, "", errors.New("文件超过 48 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(f, analyzer.MaxFileSize+1))
	return data, h.Filename, err
}
func option(v string, values map[string]string) (string, bool) { out, ok := values[v]; return out, ok }
func (s *Server) analyze(w http.ResponseWriter, r *http.Request) {
	if !multipart(w, r, 49<<20) {
		return
	}
	defer r.MultipartForm.RemoveAll()
	order, ok := option(r.FormValue("is_sort"), map[string]string{"": "desc", "0": "none", "1": "desc", "desc": "desc", "asc": "asc", "none": "none"})
	if !ok {
		fail(w, 400, "排序参数无效")
		return
	}
	mode, ok := option(r.FormValue("is_high_rank_cate"), map[string]string{"": "top12", "0": "classic", "1": "top12", "top12": "top12", "classic": "classic"})
	if !ok {
		fail(w, 400, "分类参数无效")
		return
	}
	data, name, err := upload(r, "file")
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	p, err := analyzer.Read(data, name)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	report, err := analyzer.Analyze(p, len(data))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	token := fmt.Sprintf("%d_%s.svg", time.Now().Unix(), randomID())
	source, err := s.saveUpload(strings.TrimSuffix(token, ".svg"), data, name)
	if err != nil {
		s.internal(w, err)
		return
	}
	if err = os.WriteFile(filepath.Join(s.DataDir, "reports", token), analyzer.SVG(report, order, mode, localize.Locale(r)), 0644); err != nil {
		_ = os.Remove(source)
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "token": "/api/report-img?stamp=" + token, "report": report})
}
func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	if !multipart(w, r, 97<<20) {
		return
	}
	defer r.MultipartForm.RemoveAll()
	a, an, err := upload(r, "original")
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	b, bn, err := upload(r, "compared")
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	ap, err := analyzer.Read(a, an)
	if err != nil {
		fail(w, 400, "原作品："+err.Error())
		return
	}
	bp, err := analyzer.Read(b, bn)
	if err != nil {
		fail(w, 400, "待比较作品："+err.Error())
		return
	}
	result, err := analyzer.Compare(ap, bp, len(a), len(b))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	id := fmt.Sprintf("%d_%s", time.Now().Unix(), randomID())
	original, err := s.saveUpload(id+"_original", a, an)
	if err != nil {
		s.internal(w, err)
		return
	}
	if _, err := s.saveUpload(id+"_compared", b, bn); err != nil {
		_ = os.Remove(original)
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "data": result})
}

var reportName = regexp.MustCompile(`^[0-9]+_[a-f0-9-]+\.svg$`)
var uuidName = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("stamp")
	if !reportName.MatchString(name) {
		fail(w, 400, "报告标识无效")
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	http.ServeFile(w, r, filepath.Join(s.DataDir, "reports", name))
}
func integer(r *http.Request, key string, def, min, max int) (int, bool) {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def, true
	}
	n, err := strconv.Atoi(v)
	return n, err == nil && n >= min && n <= max
}
func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	n, ok := integer(r, "n", 5, 1, 100)
	if !ok {
		fail(w, 400, "n 必须为 1–100")
		return
	}
	items, err := s.Store.Projects(r.Context(), n)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, items)
}
func (s *Server) applications(w http.ResponseWriter, r *http.Request) {
	n, ok := integer(r, "limit", 50, 1, 100)
	offset, ok2 := integer(r, "offset", 0, 0, 1000000)
	if !ok || !ok2 {
		fail(w, 400, "分页参数无效")
		return
	}
	items, err := s.Store.Applications(r.Context(), n, offset)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, 200, items)
}
