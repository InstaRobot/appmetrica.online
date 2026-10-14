// Package httpx — общие HTTP-middleware процессов ingest, api и worker.
// IP клиента читается только в clientip.go и нигде не сохраняется: ни в логах, ни в ошибках, ни в метриках.
package httpx

import "net/http"

type Middleware func(http.Handler) http.Handler

// Chain оборачивает h в mws; первый middleware в списке — внешний (выполняется первым).
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// StatusWriter запоминает статус и число записанных байт ответа; используется и middleware вне httpx.
type StatusWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (w *StatusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.status = code
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *StatusWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// NewStatusWriter оборачивает w; статус по умолчанию — 200 (обработчик ничего не записал явно).
func NewStatusWriter(w http.ResponseWriter) *StatusWriter {
	return &StatusWriter{ResponseWriter: w, status: http.StatusOK}
}

// Status — записанный статус ответа.
func (w *StatusWriter) Status() int { return w.status }

// Unwrap даёт http.ResponseController доступ к Flush и дедлайнам исходного writer.
func (w *StatusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
