package geo_test

import (
	"bytes"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/geo"
	"appmetrica.online/backend/internal/testutil"
)

func openTestDB(t testing.TB) *geo.Reader {
	t.Helper()
	r, err := geo.Open(testutil.GeoDBPath())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, r.Close()) })
	return r
}

func TestLookup_IPv6(t *testing.T) {
	r := openTestDB(t)
	require.Equal(t, geo.Location{Country: "RU", Region: "Moscow", City: "Moscow"},
		r.Lookup(netip.MustParseAddr("2a02:6b8::1")))
	require.Equal(t, geo.Location{Country: "DE", Region: "Berlin", City: "Berlin"},
		r.Lookup(netip.MustParseAddr("2001:db8::1")))
	require.Equal(t, geo.Location{}, r.Lookup(netip.MustParseAddr("10.0.0.1")))
}

func TestLookup_IPv4(t *testing.T) {
	r := openTestDB(t)
	london := geo.Location{Country: "GB", Region: "England", City: "London"}
	for _, ip := range []string{"81.2.69.142", "81.2.69.143", "::ffff:81.2.69.142"} {
		require.Equal(t, london, r.Lookup(netip.MustParseAddr(ip)), ip)
	}
	require.Equal(t, geo.Location{Country: "AU"}, r.Lookup(netip.MustParseAddr("1.0.0.1")), "сеть только со страной")
	require.Equal(t, geo.Location{}, r.Lookup(netip.MustParseAddr("8.8.8.8")), "адреса нет в базе")
}

func TestLookup_PrivateAddressEmpty(t *testing.T) {
	r := openTestDB(t)
	for _, ip := range []string{
		"10.0.0.1", "172.16.0.1", "192.168.1.1", "127.0.0.1", "0.0.0.0", "169.254.1.1", "224.0.0.1",
		"::1", "::", "fe80::1", "fc00::1", "ff02::1", "::ffff:10.0.0.1",
	} {
		require.Equal(t, geo.Location{}, r.Lookup(netip.MustParseAddr(ip)), ip)
	}
	require.Equal(t, geo.Location{}, r.Lookup(netip.Addr{}), "нулевой адрес")
}

func TestLookup_Concurrent(t *testing.T) {
	r := openTestDB(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				if r.Lookup(netip.MustParseAddr("81.2.69.142")).Country != "GB" {
					t.Error("lookup под конкуренцией вернул не GB")
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestOpen_Errors(t *testing.T) {
	_, err := geo.Open(filepath.Join(t.TempDir(), "missing.mmdb"))
	require.Error(t, err)

	notMMDB := filepath.Join(t.TempDir(), "bad.mmdb")
	require.NoError(t, os.WriteFile(notMMDB, []byte("not a maxmind database"), 0o600))
	_, err = geo.Open(notMMDB)
	require.Error(t, err)
}

// Закоммиченная testdata/geo/test-city.mmdb совпадает с тем, что собирает `go run ./testdata/gen geo`.
func TestTestGeoDB_UpToDate(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, testutil.WriteGeoDB(&buf))
	committed, err := os.ReadFile(testutil.GeoDBPath())
	require.NoError(t, err)
	require.True(t, bytes.Equal(buf.Bytes(), committed), "перегенерируйте: go run ./testdata/gen geo")
}

func BenchmarkLookup(b *testing.B) {
	r := openTestDB(b)
	addrs := []netip.Addr{
		netip.MustParseAddr("81.2.69.142"), netip.MustParseAddr("2a02:6b8::1"), netip.MustParseAddr("8.8.8.8"),
	}
	b.ReportAllocs()
	for i := 0; b.Loop(); i++ {
		_ = r.Lookup(addrs[i%len(addrs)])
	}
}
