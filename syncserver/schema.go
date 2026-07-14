package syncserver

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS server_migrations (version INTEGER PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`,
	`CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL,
		is_admin BOOLEAN NOT NULL DEFAULT FALSE, disabled BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS refresh_tokens (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		token_hash TEXT NOT NULL UNIQUE, expires_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user ON refresh_tokens(user_id, expires_at)`,
	`CREATE TABLE IF NOT EXISTS notebooks (
		id TEXT PRIMARY KEY, owner_id TEXT NOT NULL REFERENCES users(id), name TEXT NOT NULL,
		deleted_at TIMESTAMPTZ, version BIGINT NOT NULL DEFAULT 1,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS memberships (
		notebook_id TEXT NOT NULL REFERENCES notebooks(id) ON DELETE CASCADE,
		user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		role TEXT NOT NULL CHECK(role IN ('owner','editor','viewer')),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(), PRIMARY KEY(notebook_id, user_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_memberships_user ON memberships(user_id, notebook_id)`,
	`CREATE TABLE IF NOT EXISTS entities (
		entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, owner_id TEXT NOT NULL REFERENCES users(id),
		notebook_id TEXT, payload JSONB NOT NULL, version BIGINT NOT NULL DEFAULT 1,
		deleted BOOLEAN NOT NULL DEFAULT FALSE, updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		PRIMARY KEY(entity_type, entity_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_entities_owner ON entities(owner_id, entity_type, updated_at)`,
	`CREATE INDEX IF NOT EXISTS idx_entities_notebook ON entities(notebook_id, entity_type, updated_at)`,
	`CREATE TABLE IF NOT EXISTS operations (
		operation_id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id),
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS changes (
		sequence BIGSERIAL PRIMARY KEY, owner_id TEXT NOT NULL REFERENCES users(id), notebook_id TEXT,
		entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, action TEXT NOT NULL,
		version BIGINT NOT NULL, payload JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_changes_owner ON changes(owner_id, sequence)`,
	`CREATE INDEX IF NOT EXISTS idx_changes_notebook ON changes(notebook_id, sequence)`,
	`CREATE TABLE IF NOT EXISTS tombstones (
		entity_type TEXT NOT NULL, entity_id TEXT NOT NULL, owner_id TEXT NOT NULL REFERENCES users(id),
		notebook_id TEXT, deleted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
		PRIMARY KEY(entity_type, entity_id)
	)`,
	`CREATE INDEX IF NOT EXISTS idx_tombstones_deleted ON tombstones(deleted_at)`,
	`CREATE TABLE IF NOT EXISTS invitations (
		id TEXT PRIMARY KEY, notebook_id TEXT NOT NULL REFERENCES notebooks(id) ON DELETE CASCADE,
		created_by TEXT NOT NULL REFERENCES users(id), target_username TEXT, role TEXT NOT NULL CHECK(role IN ('editor','viewer')),
		token_hash TEXT NOT NULL UNIQUE, expires_at TIMESTAMPTZ NOT NULL, accepted_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS edit_leases (
		note_id TEXT PRIMARY KEY, notebook_id TEXT NOT NULL REFERENCES notebooks(id) ON DELETE CASCADE,
		holder_id TEXT NOT NULL REFERENCES users(id), expires_at TIMESTAMPTZ NOT NULL,
		updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE TABLE IF NOT EXISTS comments (
		id TEXT PRIMARY KEY, note_id TEXT NOT NULL, notebook_id TEXT NOT NULL REFERENCES notebooks(id) ON DELETE CASCADE,
		parent_id TEXT REFERENCES comments(id) ON DELETE CASCADE, author_id TEXT NOT NULL REFERENCES users(id),
		content TEXT NOT NULL, deleted_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_comments_note ON comments(note_id, created_at)`,
	`CREATE TABLE IF NOT EXISTS notifications (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		kind TEXT NOT NULL, entity_id TEXT NOT NULL, message TEXT NOT NULL, read_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_notifications_user ON notifications(user_id, read_at, created_at DESC)`,
	`CREATE TABLE IF NOT EXISTS attachments (
		id TEXT PRIMARY KEY, notebook_id TEXT NOT NULL REFERENCES notebooks(id) ON DELETE CASCADE,
		uploader_id TEXT NOT NULL REFERENCES users(id), file_name TEXT NOT NULL, mime_type TEXT NOT NULL,
		size_bytes BIGINT NOT NULL, sha256 TEXT NOT NULL, object_key TEXT NOT NULL UNIQUE,
		completed BOOLEAN NOT NULL DEFAULT FALSE, deleted_at TIMESTAMPTZ,
		created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`,
	`CREATE INDEX IF NOT EXISTS idx_attachments_notebook ON attachments(notebook_id, created_at)`,
	`INSERT INTO server_migrations(version) VALUES (1) ON CONFLICT(version) DO NOTHING`,
	`INSERT INTO server_migrations(version) VALUES (2) ON CONFLICT(version) DO NOTHING`,
}

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, statement := range schemaStatements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("apply server schema: %w", err)
		}
	}
	return tx.Commit(ctx)
}
