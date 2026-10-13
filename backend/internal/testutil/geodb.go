package testutil

import (
	"fmt"
	"io"
	"net"
	"path/filepath"

	"github.com/maxmind/mmdbwriter"
	"github.com/maxmind/mmdbwriter/mmdbtype"
)

// GeoNetwork — сеть тестовой mmdb и её гео; пустые Region и City в базу не пишутся.
type GeoNetwork struct {
	CIDR    string
	Country string
	Region  string
	City    string
}

// GeoNetworks — содержимое testdata/geo/test-city.mmdb. Адреса 81.2.69.142 и 2001:db8::1 взяты
// из официальных тестовых данных MaxMind (22-test-strategy.md), 2a02:6b8::/32 — IPv6 Яндекса,
// 1.0.0.0/24 — сеть только со страной.
var GeoNetworks = []GeoNetwork{
	{CIDR: "81.2.69.142/31", Country: "GB", Region: "England", City: "London"},
	{CIDR: "2a02:6b8::/32", Country: "RU", Region: "Moscow", City: "Moscow"},
	{CIDR: "2001:db8::/32", Country: "DE", Region: "Berlin", City: "Berlin"},
	{CIDR: "1.0.0.0/24", Country: "AU"},
}

// geoDBBuildEpoch фиксирует build_epoch, чтобы файл собирался байт в байт одинаково.
const geoDBBuildEpoch = 1_791_849_600 // 2026-10-13T00:00:00Z

// GeoDBPath — путь к тестовой mmdb в формате DB-IP City Lite.
func GeoDBPath() string { return filepath.Join(BackendDir(), "testdata", "geo", "test-city.mmdb") }

// WriteGeoDB пишет тестовую mmdb из GeoNetworks в w. Перегенерация файла: `go run ./testdata/gen geo`.
func WriteGeoDB(w io.Writer) error {
	tree, err := mmdbwriter.New(mmdbwriter.Options{
		BuildEpoch:   geoDBBuildEpoch,
		DatabaseType: "DBIP-City-Lite",
		Description:  map[string]string{"en": "appmetrica.online test City database"},
		Languages:    []string{"en"},
		RecordSize:   24,
		// 2001:db8::/32 — сеть для документации, без флага writer её не вставит.
		IncludeReservedNetworks: true,
	})
	if err != nil {
		return fmt.Errorf("testutil: new mmdb tree: %w", err)
	}
	for _, n := range GeoNetworks {
		_, network, err := net.ParseCIDR(n.CIDR)
		if err != nil {
			return fmt.Errorf("testutil: parse %s: %w", n.CIDR, err)
		}
		if err := tree.Insert(network, geoRecord(n)); err != nil {
			return fmt.Errorf("testutil: insert %s: %w", n.CIDR, err)
		}
	}
	if _, err := tree.WriteTo(w); err != nil {
		return fmt.Errorf("testutil: write mmdb: %w", err)
	}
	return nil
}

// geoRecord — запись в схеме GeoIP2/DB-IP City: country.iso_code, subdivisions[0].names.en, city.names.en.
func geoRecord(n GeoNetwork) mmdbtype.Map {
	names := func(s string) mmdbtype.Map { return mmdbtype.Map{"en": mmdbtype.String(s)} }
	rec := mmdbtype.Map{"country": mmdbtype.Map{"iso_code": mmdbtype.String(n.Country)}}
	if n.Region != "" {
		rec["subdivisions"] = mmdbtype.Slice{mmdbtype.Map{"names": names(n.Region)}}
	}
	if n.City != "" {
		rec["city"] = mmdbtype.Map{"names": names(n.City)}
	}
	return rec
}
