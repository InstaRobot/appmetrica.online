package ingest

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	KeyCacheMaxAge               = 0
	DefaultLastUsedFlushInterval = 0
	DefaultKeyCacheRefresh       = 0
)

type KeyInfo struct {
	WriteKeyID  uuid.UUID
	AppID       uuid.UUID
	AppStatus   sqlc.AppStatus
	AppPlatform sqlc.AppPlatform
}

type KeyStore interface {
	LoadWriteKeyCache(ctx context.Context) ([]sqlc.LoadWriteKeyCacheRow, error)
	TouchWriteKeysLastUsed(ctx context.Context, arg sqlc.TouchWriteKeysLastUsedParams) error
}

type KeyCacheConfig struct {
	Store         KeyStore
	Clock         clock.Clock
	Logger        *slog.Logger
	Metrics       *Metrics
	Refresh       time.Duration
	FlushInterval time.Duration
}

type KeyCache struct{}

func NewKeyCache(KeyCacheConfig) *KeyCache               { return &KeyCache{} }
func (c *KeyCache) Refresh(context.Context) error        { return nil }
func (c *KeyCache) Loaded() bool                         { return false }
func (c *KeyCache) Lookup(string) (KeyInfo, error)       { return KeyInfo{}, nil }
func (c *KeyCache) FlushLastUsed(context.Context) error  { return nil }
func (c *KeyCache) Run(context.Context)                  {}
func (c *KeyCache) ReadyCheck() ReadyCheck               { return ReadyCheck{} }
func Authenticate(_ *KeyCache, next Processor) Processor { return next }
