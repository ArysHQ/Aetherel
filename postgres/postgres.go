package postgres

import (
	"context"
	"fmt"

	"github.com/aryshq/aetherel/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(cfg *config.Config) (*pgxpool.Pool, error) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		return nil, err
	}

	if len(cfg.Database.Schema) > 0 && cfg.Database.Schema != "public" {
		_, err = db.Exec(
			ctx,
			fmt.Sprintf("SET search_path TO %v", cfg.Database.Schema),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"cannot switch to database schema %q: %w",
				cfg.Database.Schema,
				err,
			)
		}
	}
	return db, nil
}
