CREATE TABLE website_submissions (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80),
    url TEXT NOT NULL CHECK (char_length(url) <= 2048),
    category TEXT NOT NULL CHECK (category IN ('communities','tools','developers','assets','other')),
    description TEXT NOT NULL DEFAULT '' CHECK (char_length(description) <= 300),
    icon_path TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    reviewer_notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX website_active_url ON website_submissions(url) WHERE status IN ('pending','approved');
CREATE INDEX website_review_queue ON website_submissions(created_at DESC, id);
