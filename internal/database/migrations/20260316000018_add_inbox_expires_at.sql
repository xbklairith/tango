-- +goose Up
ALTER TABLE inbox_items ADD COLUMN expires_at TIMESTAMPTZ;

CREATE INDEX idx_inbox_items_approval_expires
    ON inbox_items(expires_at)
    WHERE category = 'approval'
      AND status IN ('pending', 'acknowledged')
      AND expires_at IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_inbox_items_approval_expires;
ALTER TABLE inbox_items DROP COLUMN IF EXISTS expires_at;
