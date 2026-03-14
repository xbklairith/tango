-- +goose Up

CREATE TYPE comment_author_type AS ENUM ('agent', 'user', 'system');

CREATE TABLE issue_comments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    issue_id    UUID NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
    author_type comment_author_type NOT NULL,
    author_id   UUID NOT NULL,
    body        TEXT NOT NULL CHECK (char_length(body) >= 1),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_issue_comments_issue_id ON issue_comments (issue_id, created_at ASC);

-- +goose Down
DROP TABLE IF EXISTS issue_comments;
DROP TYPE IF EXISTS comment_author_type;
