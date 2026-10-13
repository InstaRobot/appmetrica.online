package httpx

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// Gunzip распаковывает тело с Content-Encoding: gzip. Пустое значение и identity пропускаются как есть,
// остальные кодировки — 400 unsupported_content_encoding, битый заголовок gzip — 400 invalid_json.
// Размер распакованного тела ограничивает следующий за ним BodyLimit.
func Gunzip() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			values := r.Header.Values("Content-Encoding")
			enc := ""
			if len(values) == 1 {
				enc = strings.ToLower(strings.TrimSpace(values[0]))
			}
			switch {
			case len(values) == 0 || enc == "" || enc == "identity":
				r.Header.Del("Content-Encoding")
				next.ServeHTTP(w, r)
			case enc == "gzip":
				zr, err := gzip.NewReader(r.Body)
				if err != nil {
					WriteError(w, r, http.StatusBadRequest, "invalid_json", "Тело запроса не является корректным gzip.", nil)
					return
				}
				r.Body = gzipBody{Reader: zr, body: r.Body}
				r.Header.Del("Content-Encoding")
				r.Header.Del("Content-Length")
				r.ContentLength = -1
				next.ServeHTTP(w, r)
			default:
				WriteError(w, r, http.StatusBadRequest, "unsupported_content_encoding", "Поддерживается только Content-Encoding: gzip.", nil)
			}
		})
	}
}

type gzipBody struct {
	*gzip.Reader
	body io.ReadCloser
}

func (b gzipBody) Close() error {
	_ = b.Reader.Close()
	return b.body.Close()
}
