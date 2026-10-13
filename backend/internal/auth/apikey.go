package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/time/rate"

	"appmetrica.online/backend/internal/pgstore/sqlc"
)

// Scopes ключей Read API: read — все GET /v1/*, delete — создание заявок на удаление данных.
const (
	ScopeRead   = "read"
	ScopeDelete = "delete"
)

const (
	apiKeyPrefix = "sk_"
	// apiKeyChars — символов base62 после префикса (≈ 238 бит).
	apiKeyChars = 40
	// apiKeyPrefixLen — длина api_keys.key_prefix: "sk_" + 8 символов секрета.
	apiKeyPrefixLen   = len(apiKeyPrefix) + 8
	apiKeyLabelMaxLen = 64
	// apiKeyTouchInterval — last_used_at обновляется не чаще раза в минуту (условие и в TouchApiKeyLastUsed).
	apiKeyTouchInterval = time.Minute
	// Лимит Read API на ключ (06-api-contracts.md §1.6).
	apiKeyRatePerSec = 10
	apiKeyBurst      = 30

	base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	// foreignKeyViolation — SQLSTATE нарушения внешнего ключа (api_keys.app_id → apps).
	foreignKeyViolation = "23503"
)

// ErrAppNotFound — createApiKey с app_id несуществующего приложения.
var ErrAppNotFound = errors.New("auth: app not found")

// APIKey — ключ Read API без секрета и хэша.
type APIKey struct {
	ID         uuid.UUID
	KeyPrefix  string
	Label      string
	Scopes     []string
	AppID      *uuid.UUID
	CreatedAt  time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
}

// CreateAPIKeyInput — тело createApiKey; AppID nil — ключ видит все приложения.
type CreateAPIKeyInput struct {
	Label  string
	Scopes []string
	AppID  *uuid.UUID
}

// ListAPIKeys — все ключи, включая отозванные, новые первыми.
func (s *Service) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.store.Queries().ListApiKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth: list api keys: %w", err)
	}
	keys := make([]APIKey, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, APIKey(r))
	}
	return keys, nil
}

// CreateAPIKey выпускает ключ `sk_` + 40 символов base62 и возвращает его вместе с секретом. Секрет больше
// нигде не появляется: в базе — key_prefix и HMAC-SHA256(KEY_HASH_PEPPER, secret).
func (s *Service) CreateAPIKey(ctx context.Context, in CreateAPIKeyInput) (APIKey, string, error) {
	label := strings.TrimSpace(in.Label)
	if n := utf8.RuneCountInString(label); n < 1 || n > apiKeyLabelMaxLen || !utf8.ValidString(label) {
		return APIKey{}, "", &ValidationError{Field: "label", Message: "Название ключа — от 1 до 64 символов"}
	}
	if !validScopes(in.Scopes) {
		return APIKey{}, "", &ValidationError{Field: "scopes", Message: "Выберите права ключа: read и/или delete без повторов"}
	}
	secret, err := randomBase62(apiKeyChars)
	if err != nil {
		return APIKey{}, "", err
	}
	secret = apiKeyPrefix + secret
	row, err := s.store.Queries().CreateApiKey(ctx, sqlc.CreateApiKeyParams{
		KeyPrefix: secret[:apiKeyPrefixLen],
		KeyHash:   s.hashAPIKey(secret),
		Label:     label,
		Scopes:    in.Scopes,
		AppID:     in.AppID,
	})
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation:
		return APIKey{}, "", ErrAppNotFound
	case err != nil:
		return APIKey{}, "", fmt.Errorf("auth: create api key: %w", err)
	}
	s.logger.InfoContext(ctx, "api key created", slog.String("key_prefix", row.KeyPrefix))
	return APIKey(row), secret, nil
}

// RevokeAPIKey отзывает ключ; следующий запрос с ним получает 401. Повторный отзыв ничего не меняет,
// неизвестный id — ErrNotFound.
func (s *Service) RevokeAPIKey(ctx context.Context, id uuid.UUID) error {
	q := s.store.Queries()
	now := s.clock.Now()
	row, err := q.RevokeApiKey(ctx, sqlc.RevokeApiKeyParams{ID: id, Now: &now})
	if err == nil {
		s.logger.InfoContext(ctx, "api key revoked", slog.String("key_prefix", row.KeyPrefix))
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("auth: revoke api key: %w", err)
	}
	_, err = q.GetApiKey(ctx, id)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case err != nil:
		return fmt.Errorf("auth: get api key: %w", err)
	}
	return nil
}

func validScopes(scopes []string) bool {
	if len(scopes) == 0 {
		return false
	}
	for i, sc := range scopes {
		if (sc != ScopeRead && sc != ScopeDelete) || slices.Contains(scopes[:i], sc) {
			return false
		}
	}
	return true
}

func (s *Service) hashAPIKey(secret string) []byte {
	mac := hmac.New(sha256.New, s.pepper)
	mac.Write([]byte(secret))
	return mac.Sum(nil)
}

// randomBase62 — n символов base62 из crypto/rand. Байты ≥ 248 отбрасываются: 248 = 4·62, так что остаток
// по 62 распределён равномерно.
func randomBase62(n int) (string, error) {
	out := make([]byte, 0, n)
	buf := make([]byte, n*2)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("auth: random api key: %w", err)
		}
		for _, b := range buf {
			if b < 248 && len(out) < n {
				out = append(out, base62[b%62])
			}
		}
	}
	return string(out), nil
}

// isAPIKeySecret — строка имеет вид `sk_` + 40 символов base62.
func isAPIKeySecret(v string) bool {
	rest, ok := strings.CutPrefix(v, apiKeyPrefix)
	if !ok || len(rest) != apiKeyChars {
		return false
	}
	for i := range len(rest) {
		if !strings.ContainsRune(base62, rune(rest[i])) {
			return false
		}
	}
	return true
}

type apiKeyAppKey struct{}

// APIKeyAppFrom — приложение, которым ограничен Bearer-ключ запроса (api_keys.app_id). false — запрос
// по cookie владельца или ключом без ограничения.
func APIKeyAppFrom(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(apiKeyAppKey{}).(uuid.UUID)
	return id, ok
}

// RequireAPIAccess пропускает в Read API запрос с cookie am_session (владельцу доступно всё) или с
// `Authorization: Bearer sk_…`. Ключ проверяется в Postgres на каждый запрос, поэтому отзыв действует сразу.
// Для ключа по порядку: 401 unauthenticated (нет, неизвестен, отозван, Bearer выключен
// API_PUBLIC_BEARER_ENABLED=false), 429 rate_limited (10 запросов/с, burst 30 на ключ), 404 app_not_found
// (app_id маршрута — не приложение ключа), 403 insufficient_scope (нет ни одного из scopes).
// Заголовок Authorization имеет приоритет над cookie. На 401 отвечает с WWW-Authenticate: Bearer.
func (s *Service) RequireAPIAccess(writeError ErrorWriter, scopes ...string) func(http.Handler) http.Handler {
	challenge := func(w http.ResponseWriter, r *http.Request, status int, code, message string) {
		if status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", "Bearer")
		}
		writeError(w, r, status, code, message)
	}
	session := s.RequireSession(challenge)
	return func(next http.Handler) http.Handler {
		byCookie := session(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := r.Header["Authorization"]; !ok {
				byCookie.ServeHTTP(w, r)
				return
			}
			if r, ok := s.authorizeBearer(w, r, challenge, scopes); ok {
				next.ServeHTTP(w, r)
			}
		})
	}
}

// authorizeBearer проверяет Bearer-ключ запроса и возвращает запрос с ограничением ключа в контексте;
// при отказе сам пишет ответ и возвращает false.
func (s *Service) authorizeBearer(w http.ResponseWriter, r *http.Request, writeError ErrorWriter, scopes []string) (*http.Request, bool) {
	ctx := r.Context()
	unauthenticated := func(reason string) (*http.Request, bool) {
		s.metrics.authFailures.WithLabelValues(reason).Inc()
		writeError(w, r, http.StatusUnauthorized, "unauthenticated", "Неверный или отозванный API-ключ")
		return nil, false
	}
	if !s.publicBearer {
		return unauthenticated(failureBearerDisabled)
	}
	scheme, secret, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if !strings.EqualFold(scheme, "Bearer") || !isAPIKeySecret(secret) {
		return unauthenticated(failureInvalidKey)
	}
	key, err := s.store.Queries().GetActiveApiKeyByHash(ctx, s.hashAPIKey(secret))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return unauthenticated(failureInvalidKey)
	case err != nil:
		s.logger.ErrorContext(ctx, "api key check failed", slog.String("key_prefix", secret[:apiKeyPrefixLen]), slog.Any("error", err))
		writeError(w, r, http.StatusInternalServerError, "internal_error", "Внутренняя ошибка. Повторите запрос позже.")
		return nil, false
	}

	now := s.clock.Now()
	if wait, ok := s.apiKeyLimits.allow(key.ID, now); !ok {
		s.metrics.rateLimited.Inc()
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(wait.Seconds())))))
		writeError(w, r, http.StatusTooManyRequests, "rate_limited", "Слишком много запросов с этим ключом, повторите позже")
		return nil, false
	}
	if key.AppID != nil {
		if id, err := uuid.Parse(r.PathValue("app_id")); err == nil && id != *key.AppID {
			writeError(w, r, http.StatusNotFound, "app_not_found", "Приложение не найдено")
			return nil, false
		}
		r = r.WithContext(context.WithValue(ctx, apiKeyAppKey{}, *key.AppID))
	}
	if !slices.ContainsFunc(scopes, func(sc string) bool { return slices.Contains(key.Scopes, sc) }) {
		s.metrics.authFailures.WithLabelValues(failureInsufficientScope).Inc()
		writeError(w, r, http.StatusForbidden, "insufficient_scope", "У API-ключа нет прав на эту операцию")
		return nil, false
	}
	// Отметка использования не должна ронять чтение: ошибка только логируется.
	if key.LastUsedAt == nil || now.Sub(*key.LastUsedAt) >= apiKeyTouchInterval {
		if err := s.store.Queries().TouchApiKeyLastUsed(ctx, sqlc.TouchApiKeyLastUsedParams{ID: key.ID, Now: &now}); err != nil {
			s.logger.WarnContext(ctx, "api key touch failed", slog.String("key_prefix", secret[:apiKeyPrefixLen]), slog.Any("error", err))
		}
	}
	return r, true
}

// apiKeyLimits — token bucket на каждый ключ. Записей не больше, чем выпущено ключей, поэтому без вытеснения.
type apiKeyLimits struct {
	mu sync.Mutex
	m  map[uuid.UUID]*rate.Limiter
}

// allow списывает токен ключа id; при отказе возвращает время до появления токена.
func (l *apiKeyLimits) allow(id uuid.UUID, now time.Time) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim := l.m[id]
	if lim == nil {
		if l.m == nil {
			l.m = make(map[uuid.UUID]*rate.Limiter)
		}
		lim = rate.NewLimiter(apiKeyRatePerSec, apiKeyBurst)
		l.m[id] = lim
	}
	if lim.AllowN(now, 1) {
		return 0, true
	}
	return time.Duration((1 - lim.TokensAt(now)) / apiKeyRatePerSec * float64(time.Second)), false
}

// Причины api_auth_failures_total{reason}.
const (
	failureInvalidKey        = "invalid_key"
	failureBearerDisabled    = "bearer_disabled"
	failureInsufficientScope = "insufficient_scope"
)

// apiMetrics — метрики Bearer-аутентификации api (02-tech-spec.md, наблюдаемость).
type apiMetrics struct {
	// authFailures — api_auth_failures_total{reason}: отказы Bearer-запросам (401 и 403).
	authFailures *prometheus.CounterVec
	// rateLimited — api_rate_limited_total: Bearer-запросы с 429.
	rateLimited prometheus.Counter
}

func newAPIMetrics() *apiMetrics {
	m := &apiMetrics{
		authFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "api_auth_failures_total",
			Help: "Отказы Bearer-запросам Read API по причине.",
		}, []string{"reason"}),
		rateLimited: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "api_rate_limited_total",
			Help: "Запросы Read API, отклонённые лимитом ключа (429).",
		}),
	}
	for _, reason := range []string{failureInvalidKey, failureBearerDisabled, failureInsufficientScope} {
		m.authFailures.WithLabelValues(reason)
	}
	return m
}

// RegisterMetrics регистрирует api_auth_failures_total и api_rate_limited_total в reg.
func (s *Service) RegisterMetrics(reg prometheus.Registerer) error {
	for _, c := range []prometheus.Collector{s.metrics.authFailures, s.metrics.rateLimited} {
		if err := reg.Register(c); err != nil {
			return fmt.Errorf("auth: register metrics: %w", err)
		}
	}
	return nil
}
