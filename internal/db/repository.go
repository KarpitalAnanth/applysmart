package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"applysmart/internal/dedup"
	"applysmart/internal/provider"
)

const insertHitSQL = `
INSERT INTO hits (title, company, location, description, source_url, content_hash)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (content_hash) DO NOTHING`

// SaveHits inserts jobs into the hits table in a single round trip, skipping
// any whose content hash already exists. Duplicates are inserted once due to unique constraint in db.
func SaveHits(ctx context.Context, pool *pgxpool.Pool, jobs []provider.RawJob) (int, error) {
	if len(jobs) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, job := range jobs {
		batch.Queue(insertHitSQL,
			job.Title,
			job.Company,
			job.Location,
			job.Description,
			job.SourceURL,
			dedup.GenerateContentHash(job),
		)
	}

	results := pool.SendBatch(ctx, batch)

	inserted := 0
	for i := range jobs {
		tag, err := results.Exec()
		if err != nil {
			_ = results.Close()
			return 0, fmt.Errorf("db: insert hit %d (%q at %q): %w", i, jobs[i].Title, jobs[i].Company, err)
		}
		inserted += int(tag.RowsAffected())
	}

	if err := results.Close(); err != nil {
		return 0, fmt.Errorf("db: close hits batch: %w", err)
	}
	return inserted, nil
}
