-- +goose Up
-- Initial migration: create extensions and verify connectivity.
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- +goose Down
DROP EXTENSION IF EXISTS "pgcrypto";
