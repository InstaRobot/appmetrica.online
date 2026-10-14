package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/pgstore/sqlc"
)

const (
	// KeyCacheMaxAge — максимальный возраст снимка ключей при недоступном Postgres; старше — 503,
	// потому что отзыв ключа и архивирование приложения уже нельзя гарантировать (03-architecture.md).
	KeyCacheMaxAge = 15 * time.Minute
	// DefaultKeyCacheRefresh — период полной перезагрузки снимка (KEY_CACHE_REFRESH).
	DefaultKeyCacheRefresh = 15 * time.Second
	// DefaultLastUsedFlushInterval — период сброса накопленного last_used_at в Postgres.
	DefaultLastUsedFlushInterval = 5 * time.Minute

	keyCacheRetryAfter = 60
	flushTimeout       = 10 * time.Second
	// refreshTimeout — предел одной загрузки снимка: зависший запрос не должен останавливать цикл Run.
	refreshTimeout = 10 * time.Second
)

// KeyInfo — запись кэша по write-ключу pk_.
type KeyInfo struct {
	WriteKeyID  uuid.UUID
	AppID       uuid.UUID
	AppStatus   sqlc.AppStatus
	AppPlatform sqlc.AppPlatform
}

// KeyStore — запросы Postgres, нужные кэшу (реализует *sqlc.Queries).
type KeyStore interface {
	LoadWriteKeyCache(ctx context.Context) ([]sqlc.LoadWriteKeyCacheRow, error)
	TouchWriteKeysLastUsed(ctx context.Context, arg sqlc.TouchWriteKeysLastUsedParams) error
}

// KeyCacheConfig — зависимости кэша. Нулевые Refresh и FlushInterval заменяются значениями по умолчанию.
type KeyCacheConfig struct {
	Store         KeyStore
	Clock         clock.Clock
	Logger        *slog.Logger
	Metrics       *Metrics
	Refresh       time.Duration
	FlushInterval time.Duration
}

// keySnapshot — неизменяемый снимок write_keys + apps; заменяется целиком.
type keySnapshot struct {
	keys     map[string]KeyInfo
	loadedAt time.Time
}

// KeyCache — кэш write-ключей ingest. Поиск ключа — чтение map из atomic.Pointer без блокировок;
// last_used_at копится в памяти и уходит в Postgres одним UPDATE раз в FlushInterval.
type KeyCache struct {
	cfg  KeyCacheConfig
	snap atomic.Pointer[keySnapshot]

	mu   sync.Mutex
	used map[uuid.UUID]time.Time
}

func NewKeyCache(cfg KeyCacheConfig) *KeyCache {
	if cfg.Refresh <= 0 {
		cfg.Refresh = DefaultKeyCacheRefresh
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = DefaultLastUsedFlushInterval
	}
	return &KeyCache{cfg: cfg, used: map[uuid.UUID]time.Time{}}
}

// Refresh полностью перезагружает снимок из Postgres. При ошибке остаётся прежний снимок,
// ошибка логируется и считается в ingest_key_cache_refresh_errors_total. Возраст снимка отсчитывается от
// начала чтения, а не от его конца: ключ, отозванный во время долгого запроса, в снимке ещё есть, и срок его
// жизни KeyCacheMaxAge (на который опирается deletion.AppPurgeDelay) не должен удлиняться на время чтения.
func (c *KeyCache) Refresh(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()
	now := c.cfg.Clock.Now()
	rows, err := c.cfg.Store.LoadWriteKeyCache(ctx)
	if err != nil {
		c.cfg.Metrics.KeyCacheRefreshErrors.Inc()
		c.cfg.Logger.ErrorContext(ctx, "key cache refresh failed", slog.Any("error", err))
		return fmt.Errorf("ingest: load write keys: %w", err)
	}
	keys := make(map[string]KeyInfo, len(rows))
	for _, r := range rows {
		info := KeyInfo{WriteKeyID: r.WriteKeyID, AppID: r.AppID, AppStatus: r.AppStatus}
		// Пока платформа события берётся из основной платформы приложения (platforms[1], CHECK в схеме).
		if len(r.AppPlatforms) > 0 {
			info.AppPlatform = r.AppPlatforms[0]
		}
		keys[r.Key] = info
	}
	c.snap.Store(&keySnapshot{keys: keys, loadedAt: now})
	c.cfg.Metrics.KeyCacheSize.Set(float64(len(keys)))
	c.cfg.Metrics.KeyCacheLastRefresh.Set(float64(now.Unix()))
	return nil
}

// fresh возвращает снимок, если он загружен и не старше KeyCacheMaxAge.
func (c *KeyCache) fresh() (*keySnapshot, bool) {
	s := c.snap.Load()
	if s == nil || c.cfg.Clock.Now().Sub(s.loadedAt) > KeyCacheMaxAge {
		return nil, false
	}
	return s, true
}

// Lookup проверяет ключ: 503 storage_unavailable без актуального снимка, 401 invalid_app_key для
// неизвестного или отозванного ключа, 403 app_archived для архивного приложения. Ошибка — *Error.
func (c *KeyCache) Lookup(key string) (KeyInfo, error) {
	s, ok := c.fresh()
	if !ok {
		return KeyInfo{}, errKeyCacheUnavailable
	}
	info, ok := s.keys[key]
	if !ok {
		return KeyInfo{}, errInvalidAppKey
	}
	if info.AppStatus == sqlc.AppStatusArchived {
		return KeyInfo{}, errAppArchived
	}
	now := c.cfg.Clock.Now()
	c.mu.Lock()
	if now.After(c.used[info.WriteKeyID]) {
		c.used[info.WriteKeyID] = now
	}
	c.mu.Unlock()
	return info, nil
}

// Peek возвращает запись ключа из актуального снимка без проверки статуса приложения и без учёта
// last_used_at — для журнала отказов, а не для приёма событий. Снимок старше KeyCacheMaxAge не годится и
// журналу: за это время ключ могли отозвать, а приложение — удалить (deletion.AppPurgeDelay рассчитан на
// то, что после него ни один ingest не пишет строки приложения ни в events, ни в rejected_events).
func (c *KeyCache) Peek(key string) (KeyInfo, bool) {
	s, ok := c.fresh()
	if !ok {
		return KeyInfo{}, false
	}
	info, ok := s.keys[key]
	return info, ok
}

// FlushLastUsed пишет накопленный last_used_at одним запросом. При ошибке записи значения
// возвращаются в буфер и уйдут со следующим сбросом.
func (c *KeyCache) FlushLastUsed(ctx context.Context) error {
	c.mu.Lock()
	pending := c.used
	c.used = map[uuid.UUID]time.Time{}
	c.mu.Unlock()
	if len(pending) == 0 {
		return nil
	}

	ids := slices.SortedFunc(maps.Keys(pending), func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	times := make([]time.Time, len(ids))
	for i, id := range ids {
		times[i] = pending[id]
	}
	err := c.cfg.Store.TouchWriteKeysLastUsed(ctx, sqlc.TouchWriteKeysLastUsedParams{Ids: ids, LastUsed: times})
	if err == nil {
		return nil
	}
	c.mu.Lock()
	for id, t := range pending {
		if t.After(c.used[id]) {
			c.used[id] = t
		}
	}
	c.mu.Unlock()
	return fmt.Errorf("ingest: flush write key last_used_at: %w", err)
}

// Run загружает снимок сразу, затем обновляет его каждые Refresh и сбрасывает last_used_at каждые
// FlushInterval до отмены ctx; при остановке выполняет последний сброс.
func (c *KeyCache) Run(ctx context.Context) {
	_ = c.Refresh(ctx)
	refresh := time.NewTicker(c.cfg.Refresh)
	defer refresh.Stop()
	flush := time.NewTicker(c.cfg.FlushInterval)
	defer flush.Stop()
	for {
		select {
		case <-ctx.Done():
			stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), flushTimeout)
			c.flushLogged(stopCtx)
			cancel()
			return
		case <-refresh.C:
			_ = c.Refresh(ctx)
		case <-flush.C:
			c.flushLogged(ctx)
		}
	}
}

func (c *KeyCache) flushLogged(ctx context.Context) {
	if err := c.FlushLastUsed(ctx); err != nil {
		c.cfg.Logger.WarnContext(ctx, "write key last_used_at flush failed", slog.Any("error", err))
	}
}

// ReadyCheck — проверка key_cache для /ready: снимок загружен и не старше KeyCacheMaxAge.
func (c *KeyCache) ReadyCheck() ReadyCheck {
	return ReadyCheck{Name: "key_cache", Check: func(context.Context) error {
		if _, ok := c.fresh(); !ok {
			return errors.New("key cache not loaded or stale")
		}
		return nil
	}}
}

var (
	errInvalidAppKey = &Error{
		Status: http.StatusUnauthorized, Code: "invalid_app_key",
		Message: "Ключ не найден или отозван.",
	}
	errAppArchived = &Error{
		Status: http.StatusForbidden, Code: "app_archived",
		Message: "Приложение в архиве, приём событий остановлен.",
	}
	errKeyCacheUnavailable = &Error{
		Status: http.StatusServiceUnavailable, Code: "storage_unavailable",
		Message: "Хранилище временно недоступно. Повторите весь батч.", RetryAfter: keyCacheRetryAfter,
	}
)
