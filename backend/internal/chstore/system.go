package chstore

import (
	"context"
	"errors"
)

var errNotImplemented = errors.New("chstore: not implemented")

// DiskFreePct — свободное место на диске default в процентах (system.disks).
func (s *Store) DiskFreePct(_ context.Context) (float64, error) { return 0, errNotImplemented }

// EventsRows — строк в активных частях таблицы events текущей базы (system.parts).
func (s *Store) EventsRows(_ context.Context) (int64, error) { return 0, errNotImplemented }
