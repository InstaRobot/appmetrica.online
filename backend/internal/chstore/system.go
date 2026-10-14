package chstore

import (
	"context"
	"errors"
	"fmt"
)

// DiskFreePct — свободное место на диске default в процентах (system.disks).
func (s *Store) DiskFreePct(ctx context.Context) (float64, error) {
	var free, total uint64
	if err := s.conn.QueryRow(ctx,
		`SELECT free_space, total_space FROM system.disks WHERE name = 'default'`).Scan(&free, &total); err != nil {
		return 0, fmt.Errorf("chstore: disk free: %w", err)
	}
	if total == 0 {
		return 0, errors.New("chstore: disk free: total_space is 0")
	}
	return float64(free) / float64(total) * 100, nil
}

// EventsRows — строк в активных частях таблицы events текущей базы (system.parts); до слияния частей
// ReplacingMergeTree в счёт входят и дубликаты.
func (s *Store) EventsRows(ctx context.Context) (int64, error) {
	var rows uint64
	if err := s.conn.QueryRow(ctx,
		`SELECT sum(rows) FROM system.parts WHERE active AND database = currentDatabase() AND table = 'events'`).Scan(&rows); err != nil {
		return 0, fmt.Errorf("chstore: events rows: %w", err)
	}
	return int64(rows), nil //nolint:gosec // строк в таблице меньше 2^63
}
