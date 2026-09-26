package httpapi

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRetentionKeepsReportsAndRemovesOnlyExpiredSources(t *testing.T) {
	dir := t.TempDir()
	s := New(nil, dir, "")
	now := time.Now()
	oldID := "100_00000000-0000-4000-8000-000000000001"
	newID := "200_00000000-0000-4000-8000-000000000002"
	old, err := s.saveUpload(oldID, []byte("source"), "../../Project.SB3")
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.saveUpload(newID, []byte("source"), "project.json")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(old); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("source must be private", err)
	}
	if _, err := s.saveUpload(oldID, []byte("overwrite"), "project.sb3"); err == nil {
		t.Fatal("source collision must not overwrite")
	}
	paths := []string{filepath.Join(dir, "reports", oldID+".svg"), filepath.Join(dir, "media", "cover.png"), filepath.Join(dir, "uploads", "unknown.txt")}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-365*24*time.Hour), now.Add(-365*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	boundary := now.Add(-uploadRetention)
	if err := os.Chtimes(old, boundary, boundary); err != nil {
		t.Fatal(err)
	}
	s.cleanupUploads(now)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("expired source was not removed", err)
	}
	for _, path := range append(paths, current) {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("retained file %s: %v", path, err)
		}
	}
}
