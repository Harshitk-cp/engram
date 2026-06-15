package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// embeddingVectorColumns are every (table, column, index) triple that stores a
// pgvector embedding. Kept in sync with the migrations.
var embeddingVectorColumns = []struct{ table, column, index string }{
	{"memories", "embedding", "idx_memories_embedding"},
	{"episodes", "embedding", "idx_episodes_embedding"},
	{"procedures", "trigger_embedding", "idx_procedures_trigger_embedding"},
	{"schemas", "embedding", "idx_schemas_embedding"},
	{"entities", "embedding", "idx_entity_embedding"},
}

// hnswMaxDim is pgvector's hard limit for an hnsw index (vectors wider than this
// are stored but left unindexed — recall falls back to a sequential scan).
const hnswMaxDim = 2000

// EnsureEmbeddingDimension reconciles the vector columns with the configured
// embedding dimension. It is a no-op when they already match. When they differ
// AND no embeddings exist yet (a fresh deployment), it resizes the columns and
// rebuilds their indexes. When they differ but data exists, it returns an error
// rather than silently corrupting recall — the operator must re-embed instead.
func EnsureEmbeddingDimension(ctx context.Context, pool *pgxpool.Pool, wantDim int, logger *zap.Logger) error {
	if wantDim <= 0 {
		return nil
	}

	currentDim, err := vectorColumnDim(ctx, pool, "memories", "embedding")
	if err != nil {
		return fmt.Errorf("read current embedding dimension: %w", err)
	}
	if currentDim == wantDim {
		return nil
	}

	// Refuse to resize a column that holds data — that would invalidate every
	// stored vector. Operator must start fresh or run a re-embed at the new dim.
	var existing int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM memories WHERE embedding IS NOT NULL`).Scan(&existing); err != nil {
		return fmt.Errorf("count existing embeddings: %w", err)
	}
	if existing > 0 {
		return fmt.Errorf("EMBEDDING_DIM=%d does not match the existing schema dimension %d and %d embeddings already exist; "+
			"resizing would corrupt stored vectors — re-embed into a fresh database or set EMBEDDING_DIM=%d",
			wantDim, currentDim, existing, currentDim)
	}

	logger.Warn("resizing embedding columns to configured dimension",
		zap.Int("from", currentDim), zap.Int("to", wantDim))

	for _, c := range embeddingVectorColumns {
		if _, err := pool.Exec(ctx, fmt.Sprintf(`DROP INDEX IF EXISTS %s`, c.index)); err != nil {
			return fmt.Errorf("drop index %s: %w", c.index, err)
		}
		if _, err := pool.Exec(ctx, fmt.Sprintf(
			`ALTER TABLE %s ALTER COLUMN %s TYPE vector(%d)`, c.table, c.column, wantDim)); err != nil {
			return fmt.Errorf("resize %s.%s: %w", c.table, c.column, err)
		}
		if wantDim <= hnswMaxDim {
			if _, err := pool.Exec(ctx, fmt.Sprintf(
				`CREATE INDEX %s ON %s USING hnsw (%s vector_cosine_ops)`, c.index, c.table, c.column)); err != nil {
				return fmt.Errorf("recreate index %s: %w", c.index, err)
			}
		} else {
			logger.Warn("embedding dimension exceeds hnsw limit; column left unindexed (recall uses sequential scan)",
				zap.String("table", c.table), zap.Int("dim", wantDim), zap.Int("hnsw_max", hnswMaxDim))
		}
	}

	logger.Info("embedding columns resized", zap.Int("dimension", wantDim))
	return nil
}

// vectorColumnDim returns the declared dimension of a pgvector column. For the
// vector type, atttypmod is the dimension directly (no -4 offset).
func vectorColumnDim(ctx context.Context, pool *pgxpool.Pool, table, column string) (int, error) {
	var dim int
	err := pool.QueryRow(ctx,
		`SELECT atttypmod FROM pg_attribute
		 WHERE attrelid = $1::regclass AND attname = $2 AND NOT attisdropped`,
		table, column,
	).Scan(&dim)
	return dim, err
}
