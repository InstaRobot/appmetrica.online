//go:build integration

package apps_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/ingest"
	"appmetrica.online/backend/internal/pgstore"
	"appmetrica.online/backend/internal/pgstore/sqlc"
	"appmetrica.online/backend/internal/testutil"
)

const testOrigin = "http://localhost:3000"

var (
	t0           = time.Date(2026, 10, 27, 10, 0, 0, 0, time.UTC)
	writeKeyRe   = regexp.MustCompile(`^pk_[A-Za-z0-9]{32}$`)
	notesIOSBody = map[string]any{"name": "Notes iOS", "platform": "ios", "timezone": "Europe/Moscow"}
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m))
}

// env — admin API и Read API поверх настоящего Postgres с вошедшим владельцем.
type env struct {
	pg    *pgstore.Store
	clock *clock.Fixed
	mux   *http.ServeMux
	token string // am_session
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := t.Context()
	pg, err := pgstore.Open(ctx, testutil.NewPostgresDB(t, true))
	require.NoError(t, err)
	t.Cleanup(pg.Close)

	cfg := config.API{
		PublicBaseURL:     testOrigin,
		WebAuthnRPID:      "localhost",
		WebAuthnRPName:    "appmetrica.online",
		WebAuthnRPOrigins: []string{testOrigin},
		SessionTTL:        720 * time.Hour,
		SessionIdleTTL:    168 * time.Hour,
	}
	logger := slog.New(slog.DiscardHandler)
	clk := clock.NewFixed(t0)
	authSvc, err := auth.NewService(pg, clk, logger, cfg)
	require.NoError(t, err)
	mux := http.NewServeMux()
	api.NewServer(authSvc, apps.NewService(pg, clk, logger), logger, cfg).Register(mux)

	// Владелец проходит первичную настройку passkey и получает сессию.
	link, err := authSvc.CreatePasskeyLink(ctx)
	require.NoError(t, err)
	setupToken, ok := strings.CutPrefix(link, testOrigin+"/setup/")
	require.True(t, ok, link)
	c, err := authSvc.BeginSetup(ctx, setupToken)
	require.NoError(t, err)
	sess, err := authSvc.FinishSetup(ctx, auth.FinishSetupInput{
		Token:      setupToken,
		CeremonyID: c.ID,
		Credential: testutil.NewAuthenticator(t, "localhost", testOrigin).Register(t, c.Options),
		Name:       "MacBook",
		UserAgent:  "Mozilla/5.0 test",
	})
	require.NoError(t, err)
	return &env{pg: pg, clock: clk, mux: mux, token: sess.Token}
}

// call — запрос дашборда: cookie сессии и X-Requested-With. body — nil, сырая строка или значение для JSON.
func (e *env) call(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return e.callWith(t, method, path, body, func(r *http.Request) {
		r.Header.Set("X-Requested-With", auth.RequestedWith)
		r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: e.token})
	})
}

func (e *env) callWith(t *testing.T, method, path string, body any, prepare func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	var raw string
	switch b := body.(type) {
	case nil:
	case string:
		raw = b
	default:
		enc, err := json.Marshal(b)
		require.NoError(t, err)
		raw = string(enc)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(raw))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	prepare(req)
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

type appJSON struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Platform   string     `json:"platform"`
	Timezone   string     `json:"timezone"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ArchivedAt *time.Time `json:"archived_at"`
}

type writeKeyJSON struct {
	ID         string     `json:"id"`
	AppID      string     `json:"app_id"`
	Key        string     `json:"key"`
	Label      string     `json:"label"`
	RevokedAt  *time.Time `json:"revoked_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type deletionJSON struct {
	ID           string     `json:"id"`
	AppID        string     `json:"app_id"`
	SubjectType  string     `json:"subject_type"`
	SubjectValue string     `json:"subject_value"`
	Status       string     `json:"status"`
	Source       string     `json:"source"`
	RequestedAt  time.Time  `json:"requested_at"`
	StartedAt    *time.Time `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at"`
	Error        *string    `json:"error"`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}

// create создаёт приложение через POST /admin/v1/apps и требует 201.
func (e *env) create(t *testing.T, name, platform, tz string) (appJSON, writeKeyJSON) {
	t.Helper()
	rec := e.call(t, http.MethodPost, "/admin/v1/apps", map[string]any{"name": name, "platform": platform, "timezone": tz})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	out := decode[struct {
		App      appJSON      `json:"app"`
		WriteKey writeKeyJSON `json:"write_key"`
	}](t, rec)
	return out.App, out.WriteKey
}

func (e *env) get(t *testing.T, id string) appJSON {
	t.Helper()
	rec := e.call(t, http.MethodGet, "/v1/apps/"+id, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[appJSON](t, rec)
}

func (e *env) listNames(t *testing.T, query string) []string {
	t.Helper()
	rec := e.call(t, http.MethodGet, "/v1/apps"+query, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	list := decode[struct {
		Items []appJSON `json:"items"`
	}](t, rec)
	names := make([]string, 0, len(list.Items))
	for _, a := range list.Items {
		names = append(names, a.Name)
	}
	return names
}

func (e *env) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	require.NoError(t, e.pg.Pool().QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))
	return n
}

// keyCache — кэш write-ключей ingest, загруженный из той же базы.
func (e *env) keyCache(t *testing.T) *ingest.KeyCache {
	t.Helper()
	metrics, err := ingest.NewMetrics(prometheus.NewRegistry())
	require.NoError(t, err)
	cache := ingest.NewKeyCache(ingest.KeyCacheConfig{
		Store: e.pg.Queries(), Clock: e.clock, Logger: slog.New(slog.DiscardHandler), Metrics: metrics,
	})
	require.NoError(t, cache.Refresh(t.Context()))
	return cache
}

// requireIngestError — ingest отклоняет ключ с кодом code.
func requireIngestError(t *testing.T, cache *ingest.KeyCache, key, code string) {
	t.Helper()
	_, err := cache.Lookup(key)
	var ierr *ingest.Error
	require.ErrorAs(t, err, &ierr)
	require.Equal(t, code, ierr.Code)
}

// requireAPIError проверяет статус и код конверта ошибки и возвращает details.
func requireAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) map[string]any {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	env := decode[struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}](t, rec)
	require.Equal(t, code, env.Error.Code)
	require.NotEmpty(t, env.Error.Message)
	return env.Error.Details
}

// requireFieldError — 400 invalid_request с ошибкой поля field в details.fields.
func requireFieldError(t *testing.T, rec *httptest.ResponseRecorder, field string) {
	t.Helper()
	details := requireAPIError(t, rec, http.StatusBadRequest, "invalid_request")
	fields, ok := details["fields"].(map[string]any)
	require.True(t, ok, rec.Body.String())
	require.Contains(t, fields, field)
}

func TestCreateApp_CreatesAppAndFirstKey(t *testing.T) {
	e := newEnv(t)
	app, key := e.create(t, "Notes iOS", "ios", "Europe/Moscow")

	require.NotEmpty(t, app.ID)
	require.Equal(t, "Notes iOS", app.Name)
	require.Equal(t, "ios", app.Platform)
	require.Equal(t, "Europe/Moscow", app.Timezone)
	require.Equal(t, "active", app.Status)
	require.Nil(t, app.ArchivedAt)

	require.Regexp(t, writeKeyRe, key.Key)
	require.Equal(t, app.ID, key.AppID)
	require.Equal(t, "Основной", key.Label)
	require.Nil(t, key.RevokedAt)
	require.Nil(t, key.LastUsedAt)
	require.Equal(t, 1, e.count(t, "apps"))
	require.Equal(t, 1, e.count(t, "write_keys"))

	// Ключ принимается ingest после обновления кэша.
	info, err := e.keyCache(t).Lookup(key.Key)
	require.NoError(t, err)
	require.Equal(t, app.ID, info.AppID.String())
	require.Equal(t, key.ID, info.WriteKeyID.String())
	require.Equal(t, sqlc.AppStatusActive, info.AppStatus)
	require.Equal(t, sqlc.AppPlatformIos, info.AppPlatform)

	// Приложение сразу видно в Read API.
	require.Equal(t, app, e.get(t, app.ID))
	require.Equal(t, []string{"Notes iOS"}, e.listNames(t, ""))

	// Пробелы по краям имени обрезаются; второй ключ не совпадает с первым.
	other, otherKey := e.create(t, "  Notes Android \t", "android", "UTC")
	require.Equal(t, "Notes Android", other.Name)
	require.NotEqual(t, key.Key, otherKey.Key)
}

func TestCreateApp_Validation(t *testing.T) {
	e := newEnv(t)
	e.create(t, "Notes iOS", "ios", "Europe/Moscow")

	t.Run("duplicate name ignoring case", func(t *testing.T) {
		for _, name := range []string{"notes ios", "NOTES IOS", " Notes iOS "} {
			rec := e.call(t, http.MethodPost, "/admin/v1/apps", map[string]any{"name": name, "platform": "android", "timezone": "UTC"})
			requireAPIError(t, rec, http.StatusConflict, "app_name_taken")
		}
	})

	for _, tc := range []struct {
		name  string
		body  map[string]any
		field string
	}{
		{"unknown timezone", map[string]any{"name": "Bot", "platform": "bot", "timezone": "Mars/Base"}, "timezone"},
		{"empty timezone", map[string]any{"name": "Bot", "platform": "bot", "timezone": ""}, "timezone"},
		{"local timezone", map[string]any{"name": "Bot", "platform": "bot", "timezone": "Local"}, "timezone"},
		{"path in timezone", map[string]any{"name": "Bot", "platform": "bot", "timezone": "../../etc/passwd"}, "timezone"},
		{"unknown platform", map[string]any{"name": "Bot", "platform": "symbian", "timezone": "UTC"}, "platform"},
		{"blank name", map[string]any{"name": "   ", "platform": "bot", "timezone": "UTC"}, "name"},
		{"name too long", map[string]any{"name": strings.Repeat("я", 65), "platform": "bot", "timezone": "UTC"}, "name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireFieldError(t, e.call(t, http.MethodPost, "/admin/v1/apps", tc.body), tc.field)
		})
	}

	t.Run("malformed body", func(t *testing.T) {
		requireAPIError(t, e.call(t, http.MethodPost, "/admin/v1/apps", `{"name":`), http.StatusBadRequest, "invalid_request")
	})

	// Имя ровно из 64 символов допустимо.
	e.create(t, strings.Repeat("я", 64), "bot", "Asia/Tokyo")
	require.Equal(t, 2, e.count(t, "apps"))
	require.Equal(t, 2, e.count(t, "write_keys"))
}

func TestCreateApp_Concurrent(t *testing.T) {
	e := newEnv(t)
	codes := make([]int, 2)
	var wg sync.WaitGroup
	for i := range codes {
		wg.Go(func() {
			codes[i] = e.call(t, http.MethodPost, "/admin/v1/apps", notesIOSBody).Code
		})
	}
	wg.Wait()
	require.ElementsMatch(t, []int{http.StatusCreated, http.StatusConflict}, codes)
	require.Equal(t, 1, e.count(t, "apps"))
	require.Equal(t, 1, e.count(t, "write_keys"))
}

func TestUpdateApp(t *testing.T) {
	e := newEnv(t)
	app, _ := e.create(t, "Notes iOS", "ios", "Europe/Moscow")
	e.create(t, "Site", "web", "UTC")
	e.clock.Advance(time.Hour)

	rec := e.call(t, http.MethodPatch, "/admin/v1/apps/"+app.ID, map[string]any{"timezone": "Asia/Tokyo"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := decode[appJSON](t, rec)
	require.Equal(t, "Asia/Tokyo", got.Timezone)
	require.Equal(t, "Notes iOS", got.Name)
	require.True(t, t0.Add(time.Hour).Equal(got.UpdatedAt), got.UpdatedAt)

	// Смена регистра собственного имени — не конфликт.
	rec = e.call(t, http.MethodPatch, "/admin/v1/apps/"+app.ID, map[string]any{"name": " notes IOS "})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, "notes IOS", decode[appJSON](t, rec).Name)
	require.Equal(t, "ios", e.get(t, app.ID).Platform)

	requireAPIError(t, e.call(t, http.MethodPatch, "/admin/v1/apps/"+app.ID, map[string]any{"name": "SITE"}),
		http.StatusConflict, "app_name_taken")
	requireFieldError(t, e.call(t, http.MethodPatch, "/admin/v1/apps/"+app.ID, map[string]any{"timezone": "Mars/Base"}), "timezone")
	requireFieldError(t, e.call(t, http.MethodPatch, "/admin/v1/apps/"+app.ID, map[string]any{"name": ""}), "name")
	requireAPIError(t, e.call(t, http.MethodPatch, "/admin/v1/apps/"+app.ID, map[string]any{}),
		http.StatusBadRequest, "invalid_request")
	requireAPIError(t, e.call(t, http.MethodPatch, "/admin/v1/apps/0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f", map[string]any{"name": "X"}),
		http.StatusNotFound, "app_not_found")

	// Неудачные попытки ничего не изменили.
	cur := e.get(t, app.ID)
	require.Equal(t, "notes IOS", cur.Name)
	require.Equal(t, "Asia/Tokyo", cur.Timezone)
}

func TestArchiveApp_IdempotentAndUnarchive(t *testing.T) {
	e := newEnv(t)
	app, key := e.create(t, "Notes iOS", "ios", "Europe/Moscow")
	e.create(t, "Site", "web", "UTC")
	path := "/admin/v1/apps/" + app.ID

	rec := e.call(t, http.MethodPost, path+"/archive", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	archived := e.get(t, app.ID)
	require.Equal(t, "archived", archived.Status)
	require.NotNil(t, archived.ArchivedAt)
	require.True(t, t0.Equal(*archived.ArchivedAt))

	// Повторный архив — 204, archived_at не меняется.
	e.clock.Advance(time.Hour)
	require.Equal(t, http.StatusNoContent, e.call(t, http.MethodPost, path+"/archive", nil).Code)
	require.True(t, t0.Equal(*e.get(t, app.ID).ArchivedAt))

	require.Equal(t, []string{"Site"}, e.listNames(t, ""))
	require.Equal(t, []string{"Notes iOS", "Site"}, e.listNames(t, "?include_archived=true"))
	requireIngestError(t, e.keyCache(t), key.Key, "app_archived")

	rec = e.call(t, http.MethodPost, path+"/unarchive", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	require.Equal(t, http.StatusNoContent, e.call(t, http.MethodPost, path+"/unarchive", nil).Code)
	active := e.get(t, app.ID)
	require.Equal(t, "active", active.Status)
	require.Nil(t, active.ArchivedAt)
	_, err := e.keyCache(t).Lookup(key.Key)
	require.NoError(t, err)

	unknown := "/admin/v1/apps/0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f"
	requireAPIError(t, e.call(t, http.MethodPost, unknown+"/archive", nil), http.StatusNotFound, "app_not_found")
	requireAPIError(t, e.call(t, http.MethodPost, unknown+"/unarchive", nil), http.StatusNotFound, "app_not_found")
}

func TestListApps_SortedAndGetNotFound(t *testing.T) {
	e := newEnv(t)
	require.Empty(t, e.listNames(t, ""))
	for _, name := range []string{"beta", "Gamma", "alpha"} {
		e.create(t, name, "web", "UTC")
	}
	require.Equal(t, []string{"alpha", "beta", "Gamma"}, e.listNames(t, ""))
	require.Equal(t, []string{"alpha", "beta", "Gamma"}, e.listNames(t, "?include_archived=false"))

	requireAPIError(t, e.call(t, http.MethodGet, "/v1/apps/0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f", nil),
		http.StatusNotFound, "app_not_found")
	requireAPIError(t, e.call(t, http.MethodGet, "/v1/apps/not-a-uuid", nil), http.StatusBadRequest, "invalid_request")
}

func TestDeleteApp_ConfirmNameMismatch(t *testing.T) {
	e := newEnv(t)
	app, _ := e.create(t, "Notes iOS", "ios", "Europe/Moscow")

	for _, confirm := range []string{"Notes", "notes ios", "Notes iOS ", ""} {
		rec := e.call(t, http.MethodDelete, "/admin/v1/apps/"+app.ID, map[string]any{"confirm_name": confirm})
		requireAPIError(t, rec, http.StatusConflict, "confirm_name_mismatch")
	}
	require.Equal(t, app, e.get(t, app.ID))
	require.Equal(t, 0, e.count(t, "deletion_requests"))

	requireAPIError(t, e.call(t, http.MethodDelete, "/admin/v1/apps/0199d5f0-1a2b-7c3d-8e4f-5a6b7c8d9e0f",
		map[string]any{"confirm_name": "Notes iOS"}), http.StatusNotFound, "app_not_found")
}

func TestDeleteApp_CreatesDeletionRequest(t *testing.T) {
	e := newEnv(t)
	app, key := e.create(t, "Notes iOS", "ios", "Europe/Moscow")

	rec := e.call(t, http.MethodDelete, "/admin/v1/apps/"+app.ID, map[string]any{"confirm_name": "Notes iOS"})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	dr := decode[deletionJSON](t, rec)
	require.NotEmpty(t, dr.ID)
	require.Equal(t, app.ID, dr.AppID)
	require.Equal(t, "app", dr.SubjectType)
	require.Equal(t, app.ID, dr.SubjectValue)
	require.Equal(t, "pending", dr.Status)
	require.Equal(t, "dashboard", dr.Source)
	require.True(t, t0.Equal(dr.RequestedAt), dr.RequestedAt)
	require.Nil(t, dr.StartedAt)
	require.Nil(t, dr.CompletedAt)
	require.Nil(t, dr.Error)

	// Приложение сразу в архиве, а его write-ключи отозваны: со следующим снимком ingest не знает ключ
	// (401 invalid_app_key, а не 403 app_archived) и не пишет для него ни события, ни batch-ошибки
	// в rejected_events — иначе строки удалённого приложения появлялись бы после purge. Строку удалит worker.
	require.Equal(t, "archived", e.get(t, app.ID).Status)
	cache := e.keyCache(t)
	requireIngestError(t, cache, key.Key, "invalid_app_key")
	_, known := cache.Peek(key.Key)
	require.False(t, known, "журнал отказов не привязывает batch-ошибки к удаляемому приложению")
	rec = e.call(t, http.MethodGet, "/admin/v1/apps/"+app.ID+"/write-keys", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	for _, wk := range decode[struct {
		Items []writeKeyJSON `json:"items"`
	}](t, rec).Items {
		require.NotNil(t, wk.RevokedAt, "ключ %s отозван вместе с заявкой", wk.ID)
	}

	// Повторное удаление возвращает ту же заявку.
	e.clock.Advance(time.Minute)
	rec = e.call(t, http.MethodDelete, "/admin/v1/apps/"+app.ID, map[string]any{"confirm_name": "Notes iOS"})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	again := decode[deletionJSON](t, rec)
	require.Equal(t, dr.ID, again.ID)
	require.True(t, t0.Equal(again.RequestedAt))
	require.Equal(t, 1, e.count(t, "deletion_requests"))
}

// Пока заявка на удаление не выполнена, приложение нельзя вернуть из архива и выпустить ему ключ (409
// deletion_pending), а ключ, всё же появившийся в write_keys (гонка с отзывом в deleteApp), в снимок ingest не
// попадает: иначе ingest снова писал бы строки приложения, в том числе после purge.
func TestDeleteApp_FencesNewKeysAndUnarchive(t *testing.T) {
	e := newEnv(t)
	app, _ := e.create(t, "Notes iOS", "ios", "Europe/Moscow")
	path := "/admin/v1/apps/" + app.ID
	rec := e.call(t, http.MethodDelete, path, map[string]any{"confirm_name": "Notes iOS"})
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	requireAPIError(t, e.call(t, http.MethodPost, path+"/write-keys", map[string]any{"label": "late"}), http.StatusConflict, "deletion_pending")
	requireAPIError(t, e.call(t, http.MethodPost, path+"/unarchive", nil), http.StatusConflict, "deletion_pending")
	require.Equal(t, "archived", e.get(t, app.ID).Status)
	for _, wk := range e.listWriteKeys(t, app.ID) {
		require.NotNil(t, wk.RevokedAt, "новых активных ключей нет")
	}
	require.Equal(t, http.StatusNoContent, e.call(t, http.MethodPost, path+"/archive", nil).Code, "архивация идемпотентна и разрешена")

	raced, err := e.pg.Queries().CreateWriteKey(t.Context(), sqlc.CreateWriteKeyParams{
		AppID: uuid.MustParse(app.ID), Key: "pk_RacedRacedRacedRacedRacedRacedRa", Label: "raced",
	})
	require.NoError(t, err)
	cache := e.keyCache(t)
	requireIngestError(t, cache, raced.Key, "invalid_app_key")
	_, known := cache.Peek(raced.Key)
	require.False(t, known)

	// Заявка выполнена или снята — ограничения уходят вместе с ней.
	_, err = e.pg.Pool().Exec(t.Context(), "UPDATE deletion_requests SET status = 'failed' WHERE app_id = $1", app.ID)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, e.call(t, http.MethodPost, path+"/unarchive", nil).Code)
	e.createWriteKey(t, app.ID, map[string]any{"label": "after"})
	_, err = e.keyCache(t).Lookup(raced.Key)
	require.NoError(t, err)
}

func TestApps_RequireSessionAndCSRF(t *testing.T) {
	e := newEnv(t)
	app, _ := e.create(t, "Notes iOS", "ios", "Europe/Moscow")
	anonymous := func(r *http.Request) { r.Header.Set("X-Requested-With", auth.RequestedWith) }
	noCSRF := func(r *http.Request) { r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: e.token}) }

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/v1/apps"},
		{http.MethodGet, "/v1/apps/" + app.ID},
		{http.MethodPost, "/admin/v1/apps"},
		{http.MethodPatch, "/admin/v1/apps/" + app.ID},
		{http.MethodDelete, "/admin/v1/apps/" + app.ID},
		{http.MethodPost, "/admin/v1/apps/" + app.ID + "/archive"},
		{http.MethodPost, "/admin/v1/apps/" + app.ID + "/unarchive"},
	} {
		rec := e.callWith(t, tc.method, tc.path, notesIOSBody, anonymous)
		requireAPIError(t, rec, http.StatusUnauthorized, "unauthenticated")
		if tc.method != http.MethodGet {
			requireAPIError(t, e.callWith(t, tc.method, tc.path, notesIOSBody, noCSRF), http.StatusForbidden, "csrf_check_failed")
		}
	}
	require.Equal(t, app, e.get(t, app.ID))
	require.Equal(t, 1, e.count(t, "apps"))
}

// requested_at заявки — момент после захвата блокировки строки apps, а не начала запроса: при долгом ожидании
// блокировки заявка становилась бы видна ingest позже requested_at, снимок ключей, начатый в это окно, жил бы
// дольше deletion.AppPurgeDelay, и его строки появлялись бы в ClickHouse после purge.
func TestDeleteApp_RequestedAtAfterLockWait(t *testing.T) {
	e := newEnv(t)
	app, _ := e.create(t, "Notes iOS", "ios", "Europe/Moscow")
	appID := uuid.MustParse(app.ID)
	ctx := t.Context()

	holder, err := e.pg.Pool().Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = holder.Rollback(context.WithoutCancel(ctx)) }()
	_, err = holder.Exec(ctx, "SELECT id FROM apps WHERE id = $1 FOR UPDATE", appID)
	require.NoError(t, err)

	type result struct {
		dr  sqlc.DeletionRequest
		err error
	}
	done := make(chan result, 1)
	go func() {
		dr, err := apps.NewService(e.pg, e.clock, slog.New(slog.DiscardHandler)).Delete(ctx, appID, "Notes iOS")
		done <- result{dr: dr, err: err}
	}()
	require.Eventually(t, func() bool {
		var n int
		err := e.pg.Pool().QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity
WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&n)
		return err == nil && n > 0
	}, 5*time.Second, 10*time.Millisecond, "Delete ждёт блокировку строки apps")

	e.clock.Advance(2 * time.Minute)
	require.NoError(t, holder.Rollback(ctx))
	res := <-done
	require.NoError(t, res.err)
	require.True(t, t0.Add(2*time.Minute).Equal(res.dr.RequestedAt), res.dr.RequestedAt)
	require.Equal(t, "archived", e.get(t, app.ID).Status)
}
