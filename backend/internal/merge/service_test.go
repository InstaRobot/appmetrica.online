//go:build integration

package merge_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/apps"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/config"
	"appmetrica.online/backend/internal/httpx"
	"appmetrica.online/backend/internal/merge"
	"appmetrica.online/backend/internal/testutil"
)

const testOrigin = "http://localhost:3000"

var t0 = time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)

// env — admin API со слияниями поверх настоящих Postgres и ClickHouse и вошедший владелец.
type env struct {
	*testutil.Env
	handler http.Handler
	token   string // am_session
	doc     *openapi3.T
}

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := t.Context()
	te := testutil.Start(t)
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
	authSvc, err := auth.NewService(te.PG, clk, logger, cfg)
	require.NoError(t, err)

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

	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(testutil.BackendDir(), "..", "contracts", "openapi", "api.yaml"))
	require.NoError(t, err)
	doc.Servers = nil

	e := &env{Env: te, token: sess.Token, doc: doc}
	e.handler = e.newHandler(t, te.CH, cfg, authSvc, clk)
	return e
}

func (e *env) newHandler(t *testing.T, ch *chstore.Store, cfg config.API, authSvc *auth.Service, clk clock.Clock) http.Handler {
	t.Helper()
	logger := slog.New(slog.DiscardHandler)
	mux := http.NewServeMux()
	api.NewServer(authSvc, apps.NewService(e.PG, clk, logger), logger, cfg).
		WithMerges(merge.NewService(e.PG, ch.Conn(), clk, logger)).
		Register(mux)
	return httpx.Chain(mux, httpx.RequestID(clk))
}

// call — запрос дашборда с cookie сессии и X-Requested-With; ответ проверяется по api.yaml.
func (e *env) call(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw string
	if body != nil {
		enc, err := json.Marshal(body)
		require.NoError(t, err)
		raw = string(enc)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(raw))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Requested-With", auth.RequestedWith)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: e.token})
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	e.validate(t, req, rec)
	return rec
}

// validate — статус описан у операции api.yaml, тело соответствует схеме (форматы uuid/date проверяются).
func (e *env) validate(t *testing.T, req *http.Request, rec *httptest.ResponseRecorder) {
	t.Helper()
	router, err := legacy.NewRouter(e.doc)
	require.NoError(t, err)
	route, params, err := router.FindRoute(req)
	require.NoError(t, err, "%s %s", req.Method, req.URL)
	opts := &openapi3filter.Options{
		IncludeResponseStatus:   true,
		MultiError:              true,
		SchemaValidationOptions: []openapi3.SchemaValidationOption{openapi3.EnableFormatValidation()},
	}
	in := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: params, Route: route, Options: opts},
		Status:                 rec.Code,
		Header:                 rec.Header(),
		Options:                opts,
	}
	in.SetBodyBytes(rec.Body.Bytes())
	require.NoError(t, openapi3filter.ValidateResponse(t.Context(), in),
		"%s %s → %d %s", req.Method, req.URL, rec.Code, rec.Body.String())
}

type appJSON struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Platforms []string `json:"platforms"`
	Timezone  string   `json:"timezone"`
	Status    string   `json:"status"`
}

type mergeJSON struct {
	ID              string     `json:"id"`
	TargetAppID     string     `json:"target_app_id"`
	SourceAppID     *string    `json:"source_app_id"`
	SourceName      string     `json:"source_name"`
	SourcePlatforms []string   `json:"source_platforms"`
	Status          string     `json:"status"`
	Step            string     `json:"step"`
	CopiedEvents    int64      `json:"copied_events"`
	TotalEvents     int64      `json:"total_events"`
	Error           *string    `json:"error"`
	StartedAt       *time.Time `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &v), rec.Body.String())
	return v
}

// create создаёт приложение через POST /admin/v1/apps (ключ на каждую платформу).
func (e *env) create(t *testing.T, name string, platforms []string, tz string) appJSON {
	t.Helper()
	rec := e.call(t, http.MethodPost, "/admin/v1/apps", map[string]any{"name": name, "platforms": platforms, "timezone": tz})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	return decode[struct {
		App appJSON `json:"app"`
	}](t, rec).App
}

func (e *env) createMerge(t *testing.T, target, source appJSON) *httptest.ResponseRecorder {
	t.Helper()
	return e.call(t, http.MethodPost, "/admin/v1/apps/"+target.ID+"/merges",
		map[string]any{"source_app_id": source.ID, "confirm_name": source.Name})
}

func (e *env) setMerge(t *testing.T, id, set string) {
	t.Helper()
	_, err := e.PG.Pool().Exec(t.Context(), "UPDATE app_merges SET "+set+" WHERE id = $1", id)
	require.NoError(t, err)
}

func (e *env) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	require.NoError(t, e.PG.Pool().QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&n))
	return n
}

// requireAPIError проверяет статус и код конверта ошибки и возвращает details.
func requireAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) map[string]any {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	body := decode[struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}](t, rec)
	require.Equal(t, code, body.Error.Code)
	require.NotEmpty(t, body.Error.Message)
	return body.Error.Details
}

// events — n сессий приложения app: i-я в from + i*step, у каждой своя установка; user — user_id всех.
func events(app appJSON, platform, user string, n int, from time.Time, step time.Duration) []chstore.EventRow {
	appID := uuid.MustParse(app.ID)
	rows := make([]chstore.EventRow, 0, n)
	for i := range n {
		ts := from.Add(time.Duration(i) * step).UTC()
		rows = append(rows, chstore.EventRow{
			AppID:       appID,
			EventDate:   time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC),
			EventTime:   ts,
			ClientTime:  ts,
			SentAt:      ts,
			ReceivedAt:  ts,
			InsertID:    uuid.New(),
			Name:        "session_start",
			InstallID:   uuid.New(),
			UserID:      user,
			SessionID:   uuid.New(),
			Environment: "production",
			Platform:    platform,
		})
	}
	return rows
}

// seedSource — 800 событий источника в двух месячных партициях: 400 в августе с 2026-08-10T00:30Z
// и 400 в сентябре до 2026-09-20T23:30Z (в Europe/Moscow это уже 21 сентября).
func (e *env) seedSource(t *testing.T, source appJSON) {
	t.Helper()
	aug := events(source, "android", "u1", 400, time.Date(2026, 8, 10, 0, 30, 0, 0, time.UTC), time.Hour)
	sep := events(source, "android", "u1", 400, time.Date(2026, 9, 4, 8, 30, 0, 0, time.UTC), time.Hour)
	require.Equal(t, time.Date(2026, 9, 20, 23, 30, 0, 0, time.UTC), sep[len(sep)-1].EventTime)
	require.NoError(t, e.CH.InsertEvents(t.Context(), append(aug, sep...)))
}

func TestMergePreview(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "Europe/Moscow")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	empty := e.create(t, "Notes Web", []string{"web"}, "Europe/Moscow")
	e.seedSource(t, source)
	// События цели в предпросмотр источника не попадают.
	require.NoError(t, e.CH.InsertEvents(t.Context(), events(target, "ios", "u1", 50, time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Hour)))

	// Второй ключ источника — без платформы; отозванный ключ не считается.
	keyRec := e.call(t, http.MethodPost, "/admin/v1/apps/"+source.ID+"/write-keys", map[string]any{"label": "Любая"})
	require.Equal(t, http.StatusCreated, keyRec.Code, keyRec.Body.String())
	keyRec = e.call(t, http.MethodPost, "/admin/v1/apps/"+source.ID+"/write-keys", map[string]any{"label": "Старый"})
	require.Equal(t, http.StatusCreated, keyRec.Code, keyRec.Body.String())
	revoked := decode[struct {
		ID string `json:"id"`
	}](t, keyRec)
	rec := e.call(t, http.MethodPost, "/admin/v1/apps/"+source.ID+"/write-keys/"+revoked.ID+"/revoke", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	type previewJSON struct {
		Source struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Platforms []string `json:"platforms"`
		} `json:"source"`
		Events          int64   `json:"events"`
		FirstDay        *string `json:"first_day"`
		LastDay         *string `json:"last_day"`
		WriteKeys       int     `json:"write_keys"`
		TimezoneDiffers bool    `json:"timezone_differs"`
		TargetTimezone  string  `json:"target_timezone"`
		SourceTimezone  string  `json:"source_timezone"`
	}
	preview := "/admin/v1/apps/" + target.ID + "/merge-preview?source_app_id="

	rec = e.call(t, http.MethodGet, preview+source.ID, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := decode[previewJSON](t, rec)
	require.Equal(t, source.ID, got.Source.ID)
	require.Equal(t, "Notes Android", got.Source.Name)
	require.Equal(t, []string{"android"}, got.Source.Platforms)
	require.Equal(t, int64(800), got.Events)
	require.Equal(t, "2026-08-10", *got.FirstDay)
	require.Equal(t, "2026-09-20", *got.LastDay, "последний день — в часовом поясе источника")
	require.Equal(t, 2, got.WriteKeys)
	require.True(t, got.TimezoneDiffers)
	require.Equal(t, "Europe/Moscow", got.TargetTimezone)
	require.Equal(t, "UTC", got.SourceTimezone)

	t.Run("empty_source", func(t *testing.T) {
		rec := e.call(t, http.MethodGet, preview+empty.ID, nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		got := decode[previewJSON](t, rec)
		require.Zero(t, got.Events)
		require.Nil(t, got.FirstDay)
		require.Nil(t, got.LastDay)
		require.Equal(t, 1, got.WriteKeys)
		require.False(t, got.TimezoneDiffers)
	})

	t.Run("errors", func(t *testing.T) {
		details := requireAPIError(t, e.call(t, http.MethodGet, preview+target.ID, nil), http.StatusBadRequest, "invalid_request")
		require.Contains(t, details["fields"], "source_app_id")
		requireAPIError(t, e.call(t, http.MethodGet, preview+uuid.NewString(), nil), http.StatusNotFound, "app_not_found")
		requireAPIError(t, e.call(t, http.MethodGet, "/admin/v1/apps/"+uuid.NewString()+"/merge-preview?source_app_id="+source.ID, nil),
			http.StatusNotFound, "app_not_found")
		requireAPIError(t, e.call(t, http.MethodGet, "/admin/v1/apps/"+target.ID+"/merge-preview", nil), http.StatusBadRequest, "invalid_request")
	})
}

// ClickHouse недоступен — предпросмотр отвечает 503 storage_unavailable, а не 500.
func TestMergePreviewStorageDown(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "Europe/Moscow")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	ch, err := chstore.Open(t.Context(), e.ClickHouse)
	require.NoError(t, err)
	require.NoError(t, ch.Close())
	cfg := config.API{
		PublicBaseURL: testOrigin, WebAuthnRPID: "localhost", WebAuthnRPName: "appmetrica.online",
		WebAuthnRPOrigins: []string{testOrigin}, SessionTTL: 720 * time.Hour, SessionIdleTTL: 168 * time.Hour,
	}
	clk := clock.NewFixed(t0)
	authSvc, err := auth.NewService(e.PG, clk, slog.New(slog.DiscardHandler), cfg)
	require.NoError(t, err)
	e.handler = e.newHandler(t, ch, cfg, authSvc, clk)

	rec := e.call(t, http.MethodGet, "/admin/v1/apps/"+target.ID+"/merge-preview?source_app_id="+source.ID, nil)
	requireAPIError(t, rec, http.StatusServiceUnavailable, "storage_unavailable")
	require.NotEmpty(t, rec.Header().Get("Retry-After"))
}

func TestCreateMergeErrors(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "Europe/Moscow")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	e.seedSource(t, source)

	rec := e.createMerge(t, target, source)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	m := decode[mergeJSON](t, rec)
	require.Equal(t, target.ID, m.TargetAppID)
	require.Equal(t, source.ID, *m.SourceAppID)
	require.Equal(t, "Notes Android", m.SourceName)
	require.Equal(t, []string{"android"}, m.SourcePlatforms)
	require.Equal(t, "pending", m.Status)
	require.Equal(t, "move_keys", m.Step)
	require.Equal(t, int64(800), m.TotalEvents)
	require.Zero(t, m.CopiedEvents)
	require.Nil(t, m.Error)
	require.Nil(t, m.StartedAt)
	require.Nil(t, m.FinishedAt)

	// Каждый случай — свежая пара приложений, чтобы ошибка была ровно одна.
	n := 0
	pair := func(t *testing.T) (appJSON, appJSON) {
		t.Helper()
		n++
		suffix := string(rune('A' + n))
		return e.create(t, "Target "+suffix, []string{"ios"}, "UTC"), e.create(t, "Source "+suffix, []string{"web"}, "UTC")
	}

	t.Run("same_app", func(t *testing.T) {
		tgt, _ := pair(t)
		details := requireAPIError(t, e.createMerge(t, tgt, tgt), http.StatusBadRequest, "invalid_request")
		require.Contains(t, details["fields"], "source_app_id")
	})
	t.Run("confirm_name_mismatch", func(t *testing.T) {
		tgt, src := pair(t)
		rec := e.call(t, http.MethodPost, "/admin/v1/apps/"+tgt.ID+"/merges",
			map[string]any{"source_app_id": src.ID, "confirm_name": strings.ToLower(src.Name)})
		requireAPIError(t, rec, http.StatusConflict, "confirm_name_mismatch")
	})
	t.Run("deletion_pending", func(t *testing.T) {
		tgt, src := pair(t)
		rec := e.call(t, http.MethodDelete, "/admin/v1/apps/"+src.ID, map[string]any{"confirm_name": src.Name})
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		requireAPIError(t, e.createMerge(t, tgt, src), http.StatusConflict, "deletion_pending")
	})
	t.Run("target_archived", func(t *testing.T) {
		tgt, src := pair(t)
		require.Equal(t, http.StatusNoContent, e.call(t, http.MethodPost, "/admin/v1/apps/"+tgt.ID+"/archive", nil).Code)
		requireAPIError(t, e.createMerge(t, tgt, src), http.StatusConflict, "app_archived")
	})
	t.Run("archived_source_allowed", func(t *testing.T) {
		tgt, src := pair(t)
		require.Equal(t, http.StatusNoContent, e.call(t, http.MethodPost, "/admin/v1/apps/"+src.ID+"/archive", nil).Code)
		rec := e.createMerge(t, tgt, src)
		require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
		require.Zero(t, decode[mergeJSON](t, rec).TotalEvents)
	})
	t.Run("merge_in_progress", func(t *testing.T) {
		tgt, src := pair(t)
		// Цель уже принимает слияние, источник уже сливается в другое приложение, источник сам цель слияния.
		requireAPIError(t, e.createMerge(t, target, src), http.StatusConflict, "merge_in_progress")
		requireAPIError(t, e.createMerge(t, tgt, source), http.StatusConflict, "merge_in_progress")
		requireAPIError(t, e.createMerge(t, tgt, target), http.StatusConflict, "merge_in_progress")
		// Упавшее слияние тоже держит оба приложения.
		e.setMerge(t, m.ID, "status = 'failed', error = 'boom'")
		requireAPIError(t, e.createMerge(t, target, src), http.StatusConflict, "merge_in_progress")
		e.setMerge(t, m.ID, "status = 'pending', error = NULL")
	})
	t.Run("not_found", func(t *testing.T) {
		tgt, src := pair(t)
		rec := e.call(t, http.MethodPost, "/admin/v1/apps/"+tgt.ID+"/merges",
			map[string]any{"source_app_id": uuid.NewString(), "confirm_name": "Ghost"})
		requireAPIError(t, rec, http.StatusNotFound, "app_not_found")
		rec = e.call(t, http.MethodPost, "/admin/v1/apps/"+uuid.NewString()+"/merges",
			map[string]any{"source_app_id": src.ID, "confirm_name": src.Name})
		requireAPIError(t, rec, http.StatusNotFound, "app_not_found")
	})
	t.Run("invalid_body", func(t *testing.T) {
		tgt, src := pair(t)
		details := requireAPIError(t, e.call(t, http.MethodPost, "/admin/v1/apps/"+tgt.ID+"/merges",
			map[string]any{"confirm_name": "x"}), http.StatusBadRequest, "invalid_request")
		require.Contains(t, details["fields"], "source_app_id")
		details = requireAPIError(t, e.call(t, http.MethodPost, "/admin/v1/apps/"+tgt.ID+"/merges",
			map[string]any{"source_app_id": src.ID}), http.StatusBadRequest, "invalid_request")
		require.Contains(t, details["fields"], "confirm_name")
	})

	require.Equal(t, 2, e.count(t, "app_merges"), "ошибки не создают записей")
}

func TestRetryMerge(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "Europe/Moscow")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")
	rec := e.createMerge(t, target, source)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	m := decode[mergeJSON](t, rec)
	retry := "/admin/v1/apps/" + target.ID + "/merges/" + m.ID + "/retry"

	// Повторить можно только упавшее слияние.
	requireAPIError(t, e.call(t, http.MethodPost, retry, nil), http.StatusConflict, "merge_in_progress")

	e.setMerge(t, m.ID, "status = 'failed', step = 'copy', started_at = now(), error = 'clickhouse: connection refused'")
	rec = e.call(t, http.MethodPost, retry, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := decode[mergeJSON](t, rec)
	require.Equal(t, "pending", got.Status)
	require.Equal(t, "copy", got.Step, "повтор продолжает с сохранённого шага")
	require.Nil(t, got.Error)
	require.NotNil(t, got.StartedAt)
	var attempts int
	require.NoError(t, e.PG.Pool().QueryRow(t.Context(), "SELECT attempts FROM app_merges WHERE id = $1", m.ID).Scan(&attempts))
	require.Equal(t, 1, attempts)

	requireAPIError(t, e.call(t, http.MethodPost, retry, nil), http.StatusConflict, "merge_in_progress")
	e.setMerge(t, m.ID, "status = 'running'")
	requireAPIError(t, e.call(t, http.MethodPost, retry, nil), http.StatusConflict, "merge_in_progress")
	e.setMerge(t, m.ID, "status = 'done', step = 'done', finished_at = now()")
	requireAPIError(t, e.call(t, http.MethodPost, retry, nil), http.StatusConflict, "merge_in_progress")

	// Чужое или несуществующее слияние — 404 not_found, несуществующее приложение — 404 app_not_found.
	other := e.create(t, "Site", []string{"web"}, "UTC")
	requireAPIError(t, e.call(t, http.MethodPost, "/admin/v1/apps/"+other.ID+"/merges/"+m.ID+"/retry", nil),
		http.StatusNotFound, "not_found")
	requireAPIError(t, e.call(t, http.MethodPost, "/admin/v1/apps/"+target.ID+"/merges/"+uuid.NewString()+"/retry", nil),
		http.StatusNotFound, "not_found")
	requireAPIError(t, e.call(t, http.MethodPost, "/admin/v1/apps/"+uuid.NewString()+"/merges/"+m.ID+"/retry", nil),
		http.StatusNotFound, "app_not_found")
}

func TestListAppMerges(t *testing.T) {
	e := newEnv(t)
	target := e.create(t, "Notes iOS", []string{"ios"}, "Europe/Moscow")
	first := e.create(t, "Notes Android", []string{"android"}, "UTC")
	second := e.create(t, "Notes Web", []string{"web"}, "UTC")
	list := func(t *testing.T, app appJSON) []mergeJSON {
		t.Helper()
		rec := e.call(t, http.MethodGet, "/admin/v1/apps/"+app.ID+"/merges", nil)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return decode[struct {
			Items []mergeJSON `json:"items"`
		}](t, rec).Items
	}
	require.Empty(t, list(t, target))

	rec := e.createMerge(t, target, first)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	m1 := decode[mergeJSON](t, rec)
	// Слияние завершено: источник удалён, имя в журнале осталось.
	e.setMerge(t, m1.ID, "status = 'done', step = 'done', finished_at = now()")
	_, err := e.PG.Pool().Exec(t.Context(), "DELETE FROM apps WHERE id = $1", first.ID)
	require.NoError(t, err)

	rec = e.createMerge(t, target, second)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
	m2 := decode[mergeJSON](t, rec)

	got := list(t, target)
	require.Len(t, got, 2)
	require.Equal(t, m2.ID, got[0].ID, "новые первыми")
	require.Equal(t, m1.ID, got[1].ID)
	require.Nil(t, got[1].SourceAppID)
	require.Equal(t, "Notes Android", got[1].SourceName)
	require.Equal(t, "done", got[1].Status)
	require.NotNil(t, got[1].FinishedAt)
	require.Empty(t, list(t, second), "в списке только слияния, где приложение — цель")

	requireAPIError(t, e.call(t, http.MethodGet, "/admin/v1/apps/"+uuid.NewString()+"/merges", nil), http.StatusNotFound, "app_not_found")
}
