// Package geo определяет страну, регион и город по IP через DB-IP City Lite (mmdb, geoip2-golang/v2).
// Адрес используется только для поиска: пакет его не хранит, не логирует и не возвращает.
package geo

import (
	"fmt"
	"net/netip"

	"github.com/oschwald/geoip2-golang/v2"
)

// Location — гео адреса в виде колонок events; пустая строка — не определено.
type Location struct {
	Country string // ISO 3166-1 alpha-2
	Region  string // первый уровень subdivisions, имя на английском
	City    string // имя на английском
}

// Lookuper — поиск гео по адресу. Ошибок нет: адрес вне базы, частный или битая запись дают пустую Location.
type Lookuper interface {
	Lookup(addr netip.Addr) Location
}

// Reader — mmdb, открытая на всё время жизни процесса; потокобезопасен.
type Reader struct {
	db *geoip2.Reader
}

var _ Lookuper = (*Reader)(nil)

// Open открывает mmdb по пути GEOIP_DB_PATH. Ошибка — файла нет или это не City-база.
func Open(path string) (*Reader, error) {
	db, err := geoip2.Open(path)
	if err != nil {
		return nil, fmt.Errorf("geo: open mmdb: %w", err)
	}
	return &Reader{db: db}, nil
}

// Lookup возвращает гео адреса. Частные, loopback, link-local, multicast и нулевые адреса в базе
// не ищутся (02-tech-spec.md Story 1.4: гео пустое, warning не выдаётся).
func (r *Reader) Lookup(addr netip.Addr) Location {
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return Location{}
	}
	rec, err := r.db.City(addr)
	if err != nil {
		return Location{}
	}
	loc := Location{Country: rec.Country.ISOCode, City: rec.City.Names.English}
	if len(rec.Subdivisions) > 0 {
		loc.Region = rec.Subdivisions[0].Names.English
	}
	return loc
}

// Close освобождает базу.
func (r *Reader) Close() error {
	if err := r.db.Close(); err != nil {
		return fmt.Errorf("geo: close mmdb: %w", err)
	}
	return nil
}
