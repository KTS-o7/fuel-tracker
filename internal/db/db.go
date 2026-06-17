package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Entry struct {
	ID        int64   `json:"id"`
	Date      string  `json:"date"`
	Odometer  float64 `json:"odometer"`
	Liters    float64 `json:"liters"`
	PricePerL float64 `json:"price_per_l"`
	FuelType  string  `json:"fuel_type"`
	Notes     string  `json:"notes"`
	CreatedAt string  `json:"created_at"`
}

type Session struct {
	TokenHash string
	ExpiresAt time.Time
}

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	if path == "" {
		return nil, fmt.Errorf("db path required")
	}
	conn, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	if err := conn.PingContext(context.Background()); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	d := &DB{conn: conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return d, nil
}

func (d *DB) Close() error {
	return d.conn.Close()
}

func (d *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS entries (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			date          TEXT    NOT NULL,
			odometer      REAL    NOT NULL,
			liters        REAL    NOT NULL,
			price_per_l   REAL    NOT NULL,
			fuel_type     TEXT    NOT NULL DEFAULT 'regular',
			notes         TEXT    NOT NULL DEFAULT '',
			created_at    TEXT    NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_entries_date ON entries(date)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token_hash TEXT PRIMARY KEY,
			expires_at TEXT NOT NULL
		)`,
	}
	for _, s := range stmts {
		if _, err := d.conn.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) CreateEntry(ctx context.Context, e Entry) (int64, error) {
	if e.FuelType == "" {
		e.FuelType = "regular"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := d.conn.ExecContext(ctx,
		`INSERT INTO entries (date, odometer, liters, price_per_l, fuel_type, notes, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Date, e.Odometer, e.Liters, e.PricePerL, e.FuelType, e.Notes, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) GetEntry(ctx context.Context, id int64) (Entry, error) {
	row := d.conn.QueryRowContext(ctx,
		`SELECT id, date, odometer, liters, price_per_l, fuel_type, notes, created_at
		 FROM entries WHERE id = ?`, id)
	return scanEntry(row)
}

func (d *DB) ListEntries(ctx context.Context, month string) ([]Entry, error) {
	q := `SELECT id, date, odometer, liters, price_per_l, fuel_type, notes, created_at FROM entries`
	args := []any{}
	if month != "" {
		q += ` WHERE substr(date, 1, 7) = ?`
		args = append(args, month)
	}
	q += ` ORDER BY date DESC, id DESC`
	rows, err := d.conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) UpdateEntry(ctx context.Context, id int64, e Entry) error {
	if e.FuelType == "" {
		e.FuelType = "regular"
	}
	res, err := d.conn.ExecContext(ctx,
		`UPDATE entries SET date=?, odometer=?, liters=?, price_per_l=?, fuel_type=?, notes=? WHERE id=?`,
		e.Date, e.Odometer, e.Liters, e.PricePerL, e.FuelType, e.Notes, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) DeleteEntry(ctx context.Context, id int64) error {
	res, err := d.conn.ExecContext(ctx, `DELETE FROM entries WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) CreateSession(ctx context.Context, tokenHash string, expiresAt time.Time) error {
	_, err := d.conn.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, expires_at) VALUES (?, ?)`,
		tokenHash, expiresAt.UTC().Format(time.RFC3339))
	return err
}

func (d *DB) GetSession(ctx context.Context, tokenHash string) (Session, error) {
	var raw string
	err := d.conn.QueryRowContext(ctx,
		`SELECT token_hash, expires_at FROM sessions WHERE token_hash = ?`, tokenHash).
		Scan(&tokenHash, &raw)
	if err == sql.ErrNoRows {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	expires, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return Session{}, err
	}
	return Session{TokenHash: tokenHash, ExpiresAt: expires}, nil
}

func (d *DB) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := d.conn.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEntry(s scanner) (Entry, error) {
	var e Entry
	err := s.Scan(&e.ID, &e.Date, &e.Odometer, &e.Liters, &e.PricePerL, &e.FuelType, &e.Notes, &e.CreatedAt)
	if err == sql.ErrNoRows {
		return Entry{}, ErrNotFound
	}
	if err != nil {
		return Entry{}, err
	}
	return e, nil
}