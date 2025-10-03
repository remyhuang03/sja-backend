package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ProjectLink represents a single project link
type ProjectLink struct {
	Platform  string `json:"platform"`
	URL       string `json:"url"`
	IsDefault *bool  `json:"is_default,omitempty"`
}

// ProjectApplicationMeta represents the metadata of a project application
type ProjectApplicationMeta struct {
	ProjectName string        `json:"project_name"`
	AuthorName  string        `json:"author_name"`
	AuthorLink  string        `json:"author_link"`
	Brief       string        `json:"brief"`
	Links       []ProjectLink `json:"links"`
}

// ProjectApplication represents the complete application with metadata
type ProjectApplication struct {
	ApplicationID string                 `json:"application_id"`
	SubmittedAt   string                 `json:"submitted_at"`
	Meta          ProjectApplicationMeta `json:"meta"`
	CoverPath     string                 `json:"cover_path"`
	AvatarPath    string                 `json:"avatar_path"`
}

const (
	maxCoverSize  = 5 * 1024 * 1024 // 5MB
	maxAvatarSize = 2 * 1024 * 1024 // 2MB
	varDir        = "./var/project-apply"
)

// ProjectApplyHandler handles project application submissions
func ProjectApplyHandler(c *gin.Context) {
	// Parse multipart form
	if err := c.Request.ParseMultipartForm(10 << 20); err != nil { // 10MB max
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "无法解析表单数据",
			"errors":  []string{err.Error()},
		})
		return
	}

	// Get meta JSON string
	metaStr := c.PostForm("meta")
	if metaStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "缺少必填字段",
			"errors":  []string{"meta field is required"},
		})
		return
	}

	// Parse meta JSON
	var meta ProjectApplicationMeta
	if err := json.Unmarshal([]byte(metaStr), &meta); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "meta 字段格式错误",
			"errors":  []string{"Invalid JSON format: " + err.Error()},
		})
		return
	}

	// Validate meta fields
	errors := validateMeta(&meta)
	if len(errors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "请求参数验证失败",
			"errors":  errors,
		})
		return
	}

	// Get cover file
	coverFile, coverHeader, err := c.Request.FormFile("cover")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "缺少封面图片",
			"errors":  []string{"cover file is required"},
		})
		return
	}
	defer coverFile.Close()

	// Get avatar file
	avatarFile, avatarHeader, err := c.Request.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  "error",
			"message": "缺少头像图片",
			"errors":  []string{"avatar file is required"},
		})
		return
	}
	defer avatarFile.Close()

	// Validate file sizes
	if coverHeader.Size > maxCoverSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"status":  "error",
			"message": "封面图片过大",
			"errors":  []string{fmt.Sprintf("Cover image exceeds maximum size of %dMB", maxCoverSize/1024/1024)},
		})
		return
	}

	if avatarHeader.Size > maxAvatarSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{
			"status":  "error",
			"message": "头像图片过大",
			"errors":  []string{fmt.Sprintf("Avatar image exceeds maximum size of %dMB", maxAvatarSize/1024/1024)},
		})
		return
	}

	// Validate file types
	if !isValidImageType(coverHeader.Filename) {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"status":  "error",
			"message": "不支持的文件类型",
			"errors":  []string{"Cover image must be jpg, png, or webp format"},
		})
		return
	}

	if !isValidImageType(avatarHeader.Filename) {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"status":  "error",
			"message": "不支持的文件类型",
			"errors":  []string{"Avatar image must be jpg, png, or webp format"},
		})
		return
	}

	// Generate application ID
	applicationID := uuid.New().String()
	submittedAt := time.Now().UTC().Format(time.RFC3339)

	// Create application directory
	appDir := filepath.Join(varDir, applicationID)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "创建应用目录失败",
			"errors":  []string{"Failed to create application directory: " + err.Error()},
		})
		return
	}

	// Save cover file
	coverPath := filepath.Join(appDir, "cover"+getFileExtension(coverHeader.Filename))
	if err := saveFile(coverFile, coverPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "保存封面图片失败",
			"errors":  []string{"Failed to save cover image: " + err.Error()},
		})
		return
	}

	// Save avatar file
	avatarPath := filepath.Join(appDir, "avatar"+getFileExtension(avatarHeader.Filename))
	if err := saveFile(avatarFile, avatarPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "保存头像图片失败",
			"errors":  []string{"Failed to save avatar image: " + err.Error()},
		})
		return
	}

	// Create application object
	application := ProjectApplication{
		ApplicationID: applicationID,
		SubmittedAt:   submittedAt,
		Meta:          meta,
		CoverPath:     coverPath,
		AvatarPath:    avatarPath,
	}

	// Save metadata as JSON
	metaFilePath := filepath.Join(appDir, "meta.json")
	metaJSON, err := json.MarshalIndent(application, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "序列化元数据失败",
			"errors":  []string{"Failed to serialize metadata: " + err.Error()},
		})
		return
	}

	if err := os.WriteFile(metaFilePath, metaJSON, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  "error",
			"message": "保存元数据失败",
			"errors":  []string{"Failed to save metadata: " + err.Error()},
		})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"message": "申请提交成功，等待审核",
		"data": gin.H{
			"application_id": applicationID,
			"submitted_at":   submittedAt,
		},
	})
}

// validateMeta validates the project application metadata
func validateMeta(meta *ProjectApplicationMeta) []string {
	var errors []string

	if strings.TrimSpace(meta.ProjectName) == "" {
		errors = append(errors, "作品名称不能为空")
	}

	if strings.TrimSpace(meta.AuthorName) == "" {
		errors = append(errors, "作者名称不能为空")
	}

	if strings.TrimSpace(meta.AuthorLink) == "" {
		errors = append(errors, "作者主页链接不能为空")
	} else if !strings.HasPrefix(meta.AuthorLink, "http://") && !strings.HasPrefix(meta.AuthorLink, "https://") {
		errors = append(errors, "作者主页链接必须是有效的 URL")
	}

	if strings.TrimSpace(meta.Brief) == "" {
		errors = append(errors, "作品简介不能为空")
	} else if len([]rune(meta.Brief)) > 20 {
		errors = append(errors, "作品简介不能超过20字")
	}

	if len(meta.Links) == 0 {
		errors = append(errors, "至少需要添加一个作品链接")
	} else {
		defaultCount := 0
		for i, link := range meta.Links {
			if strings.TrimSpace(link.URL) == "" {
				errors = append(errors, fmt.Sprintf("第 %d 个链接的 URL 不能为空", i+1))
			} else if !strings.HasPrefix(link.URL, "http://") && !strings.HasPrefix(link.URL, "https://") {
				errors = append(errors, fmt.Sprintf("第 %d 个链接必须是有效的 URL", i+1))
			}

			if link.IsDefault != nil && *link.IsDefault {
				defaultCount++
			}
		}

		if defaultCount != 1 {
			errors = append(errors, "必须有且仅有一个默认链接")
		}
	}

	return errors
}

// isValidImageType checks if the file has a valid image extension
func isValidImageType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
}

// getFileExtension returns the file extension
func getFileExtension(filename string) string {
	return filepath.Ext(filename)
}

// saveFile saves the uploaded file to the specified path
func saveFile(src io.Reader, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, src)
	return err
}

// Initialize ensures the var directory exists
func init() {
	if err := os.MkdirAll(varDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create var directory: %v", err))
	}
}
