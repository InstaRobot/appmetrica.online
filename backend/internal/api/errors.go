package api

import "net/http"

// storageUnavailable — заглушка до реализации (T-034).
func storageUnavailable(error) bool { return false }

// responseError — заглушка до реализации (T-034).
func (s *Server) responseError(w http.ResponseWriter, r *http.Request, err error) {
	s.internalError(w, r, err)
}
