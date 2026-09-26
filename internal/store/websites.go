package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type Website struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Category    string `json:"category"`
	Description string `json:"description"`
	IconPath    string `json:"icon_path"`
}
type WebsiteSubmission struct {
	Website
	Status     string     `json:"status"`
	Notes      string     `json:"reviewer_notes"`
	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at"`
}

func (s *Store) CreateWebsite(ctx context.Context, w Website) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO website_submissions(id,name,url,category,description) VALUES($1,$2,$3,$4,$5)`, w.ID, w.Name, w.URL, w.Category, w.Description)
	return err
}

const websiteColumns = `id::text,name,url,category,description,icon_path,status,reviewer_notes,created_at,reviewed_at`

func scanWebsite(row pgx.Row) (WebsiteSubmission, error) {
	var w WebsiteSubmission
	err := row.Scan(&w.ID, &w.Name, &w.URL, &w.Category, &w.Description, &w.IconPath, &w.Status, &w.Notes, &w.CreatedAt, &w.ReviewedAt)
	return w, err
}
func (s *Store) Website(ctx context.Context, id string) (WebsiteSubmission, error) {
	return scanWebsite(s.Pool.QueryRow(ctx, `SELECT `+websiteColumns+` FROM website_submissions WHERE id=$1`, id))
}
func (s *Store) WebsiteSubmissions(ctx context.Context, limit, offset int) ([]WebsiteSubmission, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+websiteColumns+` FROM website_submissions ORDER BY created_at DESC,id LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WebsiteSubmission{}
	for rows.Next() {
		w, err := scanWebsite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (s *Store) Websites(ctx context.Context) ([]Website, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text,name,url,category,description,icon_path FROM website_submissions WHERE status='approved' ORDER BY reviewed_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Website{}
	for rows.Next() {
		var w Website
		if err := rows.Scan(&w.ID, &w.Name, &w.URL, &w.Category, &w.Description, &w.IconPath); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (s *Store) ReviewWebsite(ctx context.Context, id, status, notes, icon string) error {
	// A conditional update makes publication and review atomic, including concurrent reviews.
	tag, err := s.Pool.Exec(ctx, `UPDATE website_submissions SET status=$2,reviewer_notes=$3,icon_path=$4,reviewed_at=now() WHERE id=$1 AND status='pending'`, id, status, notes, icon)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}
