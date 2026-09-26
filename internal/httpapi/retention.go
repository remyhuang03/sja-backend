package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const uploadRetention = 30 * 24 * time.Hour

var uploadName = regexp.MustCompile(`^[0-9]+_[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}(_original|_compared)?\.(sb3|cc3|json)$`)

// saveUpload retains accepted source bytes privately, using a server-generated identifier.
// Analysis sources share their report's identifier; comparison sources share a pair identifier.
func (s *Server) saveUpload(id string, data []byte, originalName string) (string, error) {
	name := id + strings.ToLower(filepath.Ext(originalName))
	if !uploadName.MatchString(name) {
		return "", fmt.Errorf("invalid retained upload name")
	}
	dir := filepath.Join(s.DataDir, "uploads")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if writeErr != nil {
			return "", writeErr
		}
		return "", closeErr
	}
	return path, nil
}

// cleanupUploads never touches reports or showcase media. Unknown files and links are skipped.
func (s *Server) cleanupUploads(now time.Time) {
	dir := filepath.Join(s.DataDir, "uploads")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		slog.Error("upload cleanup", "error", err)
		return
	}
	for _, entry := range entries {
		if !uploadName.MatchString(entry.Name()) || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			slog.Error("upload metadata", "error", err)
			continue
		}
		if info.Mode().IsRegular() && !info.ModTime().Add(uploadRetention).After(now) {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				slog.Error("upload removal", "error", err)
			}
		}
	}
}

// RunCleanup removes source uploads after 30 days. Reports have no automatic expiration.
func (s *Server) RunCleanup(ctx context.Context) {
	s.cleanupUploads(time.Now())
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.cleanupUploads(now)
		}
	}
}
