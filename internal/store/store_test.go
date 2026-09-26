package store

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
)

// Integration tests use a disposable schema, even when the test DB is reused.
func TestReviewTransaction(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Pool.Close()
	_, err = s.Pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS sja_integration")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, err := Open(ctx, url)
		if err == nil {
			defer cleanup.Pool.Close()
			_, _ = cleanup.Pool.Exec(ctx, "DROP SCHEMA sja_integration CASCADE")
		}
	}()
	s.Pool.Close()
	sep := "?"
	for _, c := range url {
		if c == '?' {
			sep = "&"
			break
		}
	}
	s, err = Open(ctx, url+sep+"search_path=sja_integration")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Pool.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal("migration must be idempotent:", err)
	}
	a := Application{ID: "00000000-0000-4000-8000-000000000001", Meta: Meta{ProjectName: "测试", AuthorName: "作者", AuthorLink: "https://example.com", Brief: "测试作品", Links: []Link{{URL: "https://example.com/project", Default: true}}}, CoverPath: "/cover.png", AvatarPath: "/avatar.png"}
	if err = s.Create(ctx, a); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.Review(ctx, a.ID, "approved", "") }()
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("success %d, conflict %d", ok, conflict)
	}
	projects, err := s.Projects(ctx, 10)
	if err != nil || len(projects) != 1 || projects[0].CoverPath != "/cover.png" {
		t.Fatal(projects, err)
	}
	apps, err := s.Applications(ctx, 20, 0)
	if err != nil || len(apps) != 1 || apps[0].Status != "approved" {
		t.Fatal(apps, err)
	}
	site := Website{ID: "00000000-0000-4000-8000-000000000002", Name: "Resource", URL: "https://example.org/", Category: "tools"}
	if err := s.CreateWebsite(ctx, site); err != nil {
		t.Fatal(err)
	}
	reviews := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); reviews <- s.ReviewWebsite(ctx, site.ID, "approved", "", "") }()
	}
	wg.Wait()
	close(reviews)
	ok, conflict = 0, 0
	for err := range reviews {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal("concurrent website reviews", ok, conflict)
	}
	websites, err := s.Websites(ctx)
	if err != nil || len(websites) != 1 {
		t.Fatal(websites, err)
	}

}
