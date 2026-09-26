package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS
var ErrConflict = errors.New("申请已审核")

type Link struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
	Default  bool   `json:"is_default"`
}
type Meta struct {
	ProjectName string `json:"project_name"`
	AuthorName  string `json:"author_name"`
	AuthorLink  string `json:"author_link"`
	Brief       string `json:"brief"`
	Links       []Link `json:"links"`
}
type Application struct {
	ID string `json:"id"`
	Meta
	CoverPath  string     `json:"cover_image_path"`
	AvatarPath string     `json:"avatar_image_path"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at"`
	Notes      string     `json:"reviewer_notes"`
}
type Project struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Author      string `json:"author"`
	AuthorLink  string `json:"author_link"`
	ProjectLink string `json:"project_link"`
	Brief       string `json:"brief"`
	CoverPath   string `json:"cover_image_path"`
	AvatarPath  string `json:"avatar_image_path"`
}
type Store struct{ Pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	c, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("DATABASE_URL 无效")
	}
	c.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	c.MaxConns = 10
	c.MinConns = 1
	c.MaxConnLifetime = time.Hour
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, err
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Store{p}, nil
}
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(73821001)"); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, f := range files {
		var exists bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE name=$1)", f.Name()).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		data, err := migrations.ReadFile("migrations/" + f.Name())
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("migration %s: %w", f.Name(), err)
		}
		if _, err = tx.Exec(ctx, "INSERT INTO schema_migrations(name) VALUES($1)", f.Name()); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Create(ctx context.Context, a Application) error {
	links, err := json.Marshal(a.Links)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO applications(id,project_name,author_name,author_link,brief,links,cover_path,avatar_path) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, a.ID, a.ProjectName, a.AuthorName, a.AuthorLink, a.Brief, links, a.CoverPath, a.AvatarPath)
	return err
}
func scanApplication(row pgx.Row) (Application, error) {
	var a Application
	var links []byte
	err := row.Scan(&a.ID, &a.ProjectName, &a.AuthorName, &a.AuthorLink, &a.Brief, &links, &a.CoverPath, &a.AvatarPath, &a.Status, &a.CreatedAt, &a.ReviewedAt, &a.Notes)
	if err == nil {
		err = json.Unmarshal(links, &a.Links)
	}
	return a, err
}

const applicationColumns = `id::text,project_name,author_name,author_link,brief,links,cover_path,avatar_path,status,created_at,reviewed_at,reviewer_notes`

func (s *Store) Applications(ctx context.Context, limit, offset int) ([]Application, error) {
	rows, err := s.Pool.Query(ctx, "SELECT "+applicationColumns+" FROM applications ORDER BY created_at DESC,id LIMIT $1 OFFSET $2", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Application{}
	for rows.Next() {
		a, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (s *Store) Review(ctx context.Context, id, status, notes string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	a, err := scanApplication(tx.QueryRow(ctx, "SELECT "+applicationColumns+" FROM applications WHERE id=$1 FOR UPDATE", id))
	if err != nil {
		return err
	}
	if a.Status != "pending" {
		return ErrConflict
	}
	if status == "approved" {
		link := ""
		for _, l := range a.Links {
			if l.Default {
				link = l.URL
				break
			}
		}
		if link == "" {
			return errors.New("申请缺少默认链接")
		}
		_, err = tx.Exec(ctx, `INSERT INTO projects(application_id,name,author,author_link,project_link,brief,cover_path,avatar_path) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, a.ProjectName, a.AuthorName, a.AuthorLink, link, a.Brief, a.CoverPath, a.AvatarPath)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, "UPDATE applications SET status=$2,reviewed_at=now(),reviewer_notes=$3 WHERE id=$1", id, status, notes)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Projects(ctx context.Context, n int) ([]Project, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,name,author,author_link,project_link,brief,cover_path,avatar_path FROM projects ORDER BY created_at DESC,id DESC LIMIT $1`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		if err = rows.Scan(&p.ID, &p.Name, &p.Author, &p.AuthorLink, &p.ProjectLink, &p.Brief, &p.CoverPath, &p.AvatarPath); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
