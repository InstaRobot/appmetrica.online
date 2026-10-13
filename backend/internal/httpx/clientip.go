package httpx

import (
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP — единственное место чтения RemoteAddr и X-Forwarded-For. Возвращает последний адрес
// X-Forwarded-For (его добавляет Traefik), при его отсутствии или невалидности — адрес из RemoteAddr.
// Результат используется только в памяти для гео-поиска и не сохраняется нигде.
func ClientIP(r *http.Request) (netip.Addr, bool) {
	if addr, ok := lastForwarded(r.Header.Values("X-Forwarded-For")); ok {
		return addr, true
	}
	if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
		return normalize(ap.Addr()), true
	}
	if addr, err := netip.ParseAddr(r.RemoteAddr); err == nil {
		return normalize(addr), true
	}
	return netip.Addr{}, false
}

func lastForwarded(values []string) (netip.Addr, bool) {
	for i := len(values) - 1; i >= 0; i-- {
		parts := strings.Split(values[i], ",")
		for j := len(parts) - 1; j >= 0; j-- {
			p := strings.TrimSpace(parts[j])
			if p == "" {
				continue
			}
			addr, err := netip.ParseAddr(p)
			if err != nil {
				return netip.Addr{}, false
			}
			return normalize(addr), true
		}
	}
	return netip.Addr{}, false
}

func normalize(a netip.Addr) netip.Addr { return a.Unmap().WithZone("") }
