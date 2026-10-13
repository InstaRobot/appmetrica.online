// Package geo определяет страну, регион и город по IP через DB-IP City Lite (mmdb).
package geo

import (
	"errors"
	"net/netip"
)

// Location — гео адреса; пустые строки — не определено.
type Location struct {
	Country string
	Region  string
	City    string
}

// Reader — mmdb в памяти.
type Reader struct{}

// Open открывает mmdb.
func Open(string) (*Reader, error) { return nil, errors.New("geo: not implemented") }

// Lookup возвращает гео адреса.
func (r *Reader) Lookup(netip.Addr) Location { return Location{} }

// Close закрывает базу.
func (r *Reader) Close() error { return nil }
