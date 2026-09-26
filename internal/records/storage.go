package records

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("запись не найдена")

type Storage struct {
	db *pgxpool.Pool
}

func NewStorage(db *pgxpool.Pool) *Storage {
	return &Storage{db: db}
}

func (s *Storage) GetList(ctx context.Context) ([]Record, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name, created_at FROM records ORDER BY id`)
	if err != nil {
		return nil, err
	}
	records, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Record])
	if err != nil {
		return nil, err
	}
	if records == nil {
		records = []Record{}
	}
	return records, nil
}

func (s *Storage) GetByID(ctx context.Context, id int64) (Record, error) {
	var rec Record
	err := s.db.QueryRow(ctx,
		`SELECT id, name, created_at FROM records WHERE id = $1`,
		id,
	).Scan(&rec.ID, &rec.Name, &rec.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return rec, err
}

func (s *Storage) Create(ctx context.Context, name string) (Record, error) {
	var rec Record
	err := s.db.QueryRow(ctx,
		`INSERT INTO records (name) VALUES ($1) RETURNING id, name, created_at`,
		name,
	).Scan(&rec.ID, &rec.Name, &rec.CreatedAt)
	return rec, err
}

func (s *Storage) Update(ctx context.Context, id int64, name string) (Record, error) {
	var rec Record
	err := s.db.QueryRow(ctx,
		`UPDATE records SET name = $2 WHERE id = $1 RETURNING id, name, created_at`,
		id, name,
	).Scan(&rec.ID, &rec.Name, &rec.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	return rec, err
}

func (s *Storage) Delete(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM records WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
