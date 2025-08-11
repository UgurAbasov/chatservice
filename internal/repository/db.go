package repository

import (
	"context"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDBPool(connString string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, err
	}

	log.Println("Successfully connected to PostgreSQL database.")
	return pool, nil
}