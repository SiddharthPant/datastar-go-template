package auth

import (
	"datastar-go/database/sqlc"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	queries *sqlc.Queries
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{queries: sqlc.New(db)}
}
