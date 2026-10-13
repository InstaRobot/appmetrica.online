package ingest

import (
	"context"
	"net/http"
)

type ReadyCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

func NewReadyHandler(...ReadyCheck) http.Handler { return http.NotFoundHandler() }
