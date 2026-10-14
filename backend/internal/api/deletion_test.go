//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/auth"
	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/testutil"
)

func deletionsPath(appID uuid.UUID) string {
	return "/v1/apps/" + appID.String() + "/deletion-requests"
}

// deletionKey — отдельный ключ sk_ со scopes; appID nil — ключ видит все приложения.
func (e *portfolioEnv) deletionKey(t *testing.T, appID *uuid.UUID, scopes ...string) (auth.APIKey, string) {
	t.Helper()
	key, secret, err := e.auth.CreateAPIKey(t.Context(), auth.CreateAPIKeyInput{Label: "deletion", Scopes: scopes, AppID: appID})
	require.NoError(t, err)
	return key, secret
}

func bearer(secret string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+secret) }
}

// asOwner — запрос дашборда: cookie сессии и X-Requested-With.
func asOwner(cookie *http.Cookie) func(*http.Request) {
	return func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set("X-Requested-With", auth.RequestedWith)
	}
}

// call выполняет запрос к API. body — nil, сырая строка или значение для JSON.
func (e *portfolioEnv) call(t *testing.T, method, path string, body any, prepare func(*http.Request)) *httptest.ResponseRecorder {
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
	return serveRequest(t, e.handler, req)
}

func deletionBody(t *testing.T, rec *httptest.ResponseRecorder, status int) gen.DeletionRequest {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	var dr gen.DeletionRequest
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dr))
	return dr
}

func (e *portfolioEnv) countDeletionRequests(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, e.env.PG.Pool().QueryRow(t.Context(), `SELECT count(*) FROM deletion_requests`).Scan(&n))
	return n
}

func TestCreateDeletionRequest_Validation(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	msk := testutil.ScenarioApps[0].ID
	nyc := testutil.ScenarioApps[1].ID

	cases := []struct {
		name   string
		app    uuid.UUID
		body   any
		status int
		code   string
		field  string // details.field; "" — без поля
	}{
		{
			"install_not_uuid", msk,
			map[string]string{"subject_type": "install", "subject_value": "abc"},
			http.StatusBadRequest, "invalid_request", "subject_value",
		},
		{
			"install_nil_uuid", msk,
			map[string]string{"subject_type": "install", "subject_value": uuid.Nil.String()},
			http.StatusBadRequest, "invalid_request", "subject_value",
		},
		{
			"install_braced_uuid", msk,
			map[string]string{"subject_type": "install", "subject_value": "{" + uuid.NewString() + "}"},
			http.StatusBadRequest, "invalid_request", "subject_value",
		},
		{
			"user_empty", msk,
			map[string]string{"subject_type": "user", "subject_value": ""},
			http.StatusBadRequest, "invalid_request", "subject_value",
		},
		{
			"user_too_long", msk,
			map[string]string{"subject_type": "user", "subject_value": strings.Repeat("я", 129)},
			http.StatusBadRequest, "invalid_request", "subject_value",
		},
		{
			// Ревью: NUL проходил валидацию, а Postgres text его не хранит — был 500.
			"user_nul", msk,
			map[string]string{"subject_type": "user", "subject_value": "user\x00suffix"},
			http.StatusBadRequest, "invalid_request", "subject_value",
		},
		{
			"subject_app_rejected", msk,
			map[string]string{"subject_type": "app", "subject_value": msk.String()},
			http.StatusBadRequest, "invalid_request", "subject_type",
		},
		{
			"subject_unknown", msk,
			map[string]string{"subject_type": "device", "subject_value": "x"},
			http.StatusBadRequest, "invalid_request", "subject_type",
		},
		{
			"subject_missing", msk,
			map[string]string{"subject_value": "u1"},
			http.StatusBadRequest, "invalid_request", "subject_type",
		},
		{"body_not_json", msk, "{", http.StatusBadRequest, "invalid_request", ""},
		{
			"unknown_app", uuid.New(),
			map[string]string{"subject_type": "user", "subject_value": "u1"},
			http.StatusNotFound, "app_not_found", "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, secret := e.deletionKey(t, nil, auth.ScopeDelete)
			path := deletionsPath(tc.app)
			rec := e.call(t, http.MethodPost, path, tc.body, bearer(secret))
			requireEnvelope(t, rec, tc.status, tc.code)
			if tc.field != "" {
				require.Equal(t, tc.field, errorBody(t, rec, tc.status).Details["field"])
			}
			// Присланное значение не цитируется в сообщении.
			require.NotContains(t, rec.Body.String(), "abc")
			validateResponse(t, doc, httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, nil), rec)
		})
	}
	require.Zero(t, e.countDeletionRequests(t), "отклонённые запросы не сохраняются")

	t.Run("read_scope_only", func(t *testing.T) {
		_, secret := e.deletionKey(t, nil, auth.ScopeRead)
		rec := e.call(t, http.MethodPost, deletionsPath(msk),
			map[string]string{"subject_type": "user", "subject_value": "u1"}, bearer(secret))
		requireEnvelope(t, rec, http.StatusForbidden, "insufficient_scope")
	})
	t.Run("foreign_app_key", func(t *testing.T) {
		_, secret := e.deletionKey(t, &nyc, auth.ScopeDelete)
		rec := e.call(t, http.MethodPost, deletionsPath(msk),
			map[string]string{"subject_type": "user", "subject_value": "u1"}, bearer(secret))
		requireEnvelope(t, rec, http.StatusNotFound, "app_not_found")
	})
	t.Run("cookie_requires_csrf", func(t *testing.T) {
		cookie := ownerCookie(t, e)
		rec := e.call(t, http.MethodPost, deletionsPath(msk),
			map[string]string{"subject_type": "user", "subject_value": "u1"}, func(r *http.Request) { r.AddCookie(cookie) })
		requireEnvelope(t, rec, http.StatusForbidden, "csrf_check_failed")
	})
	require.Zero(t, e.countDeletionRequests(t))
}

func TestCreateDeletionRequest_Idempotent(t *testing.T) {
	e := newPortfolioEnv(t)
	msk := testutil.ScenarioApps[0].ID
	nyc := testutil.ScenarioApps[1].ID
	_, secret := e.deletionKey(t, nil, auth.ScopeDelete)
	u1 := map[string]string{"subject_type": "user", "subject_value": "u1"}

	first := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(msk), u1, bearer(secret)), http.StatusAccepted)
	require.Equal(t, gen.DeletionStatusPending, first.Status)

	t.Run("repeat_while_pending", func(t *testing.T) {
		again := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(msk), u1, bearer(secret)), http.StatusAccepted)
		require.Equal(t, first, again)
		require.Equal(t, 1, e.countDeletionRequests(t))
	})
	t.Run("repeat_from_dashboard_keeps_source", func(t *testing.T) {
		again := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(msk), u1, asOwner(ownerCookie(t, e))), http.StatusAccepted)
		require.Equal(t, first.Id, again.Id)
		require.Equal(t, gen.DeletionSourceApi, again.Source)
	})
	t.Run("repeat_while_running", func(t *testing.T) {
		_, err := e.env.PG.Pool().Exec(t.Context(),
			`UPDATE deletion_requests SET status = 'running', started_at = now() WHERE id = $1`, first.Id)
		require.NoError(t, err)
		again := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(msk), u1, bearer(secret)), http.StatusAccepted)
		require.Equal(t, first.Id, again.Id)
		require.Equal(t, gen.DeletionStatusRunning, again.Status)
	})
	t.Run("other_subjects_are_separate", func(t *testing.T) {
		other := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(msk),
			map[string]string{"subject_type": "user", "subject_value": "u2"}, bearer(secret)), http.StatusAccepted)
		require.NotEqual(t, first.Id, other.Id)
		otherApp := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(nyc), u1, bearer(secret)), http.StatusAccepted)
		require.NotEqual(t, first.Id, otherApp.Id)
		require.Equal(t, nyc, otherApp.AppId)
	})
	t.Run("install_value_is_canonical", func(t *testing.T) {
		install := uuid.New()
		lower := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(msk),
			map[string]string{"subject_type": "install", "subject_value": install.String()}, bearer(secret)), http.StatusAccepted)
		upper := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(msk),
			map[string]string{"subject_type": "install", "subject_value": strings.ToUpper(install.String())}, bearer(secret)), http.StatusAccepted)
		require.Equal(t, lower.Id, upper.Id)
		require.Equal(t, install.String(), upper.SubjectValue)
		require.Equal(t, gen.DeletionSubjectTypeInstall, upper.SubjectType)
	})
	t.Run("new_request_after_done", func(t *testing.T) {
		_, err := e.env.PG.Pool().Exec(t.Context(),
			`UPDATE deletion_requests SET status = 'done', completed_at = now() WHERE id = $1`, first.Id)
		require.NoError(t, err)
		next := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(msk), u1, bearer(secret)), http.StatusAccepted)
		require.NotEqual(t, first.Id, next.Id)
		require.Equal(t, gen.DeletionStatusPending, next.Status)
	})
}

func TestGetDeletionRequest(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	msk := testutil.ScenarioApps[0].ID
	nyc := testutil.ScenarioApps[1].ID
	key, secret := e.deletionKey(t, nil, auth.ScopeDelete)

	created := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(msk),
		map[string]string{"subject_type": "user", "subject_value": "u1"}, bearer(secret)), http.StatusAccepted)
	path := deletionsPath(msk) + "/" + created.Id.String()

	t.Run("by_read_key", func(t *testing.T) {
		_, read := e.deletionKey(t, nil, auth.ScopeRead)
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		bearer(read)(req)
		rec := serveRequest(t, e.handler, req)
		got := deletionBody(t, rec, http.StatusOK)
		validateResponse(t, doc, req, rec)

		require.Equal(t, created.Id, got.Id)
		require.Equal(t, msk, got.AppId)
		require.Equal(t, gen.DeletionSubjectTypeUser, got.SubjectType)
		require.Equal(t, "u1", got.SubjectValue)
		require.Equal(t, gen.DeletionStatusPending, got.Status)
		require.Equal(t, gen.DeletionSourceApi, got.Source)
		require.True(t, got.RequestedAt.Equal(e.clock.Now()), got.RequestedAt)
		require.True(t, got.StartedAt.IsNull())
		require.True(t, got.CompletedAt.IsNull())
		require.True(t, got.Error.IsNull())
	})
	t.Run("api_key_id_is_stored", func(t *testing.T) {
		var source string
		var apiKeyID *uuid.UUID
		require.NoError(t, e.env.PG.Pool().QueryRow(t.Context(),
			`SELECT source::text, api_key_id FROM deletion_requests WHERE id = $1`, created.Id).Scan(&source, &apiKeyID))
		require.Equal(t, "api", source)
		require.NotNil(t, apiKeyID)
		require.Equal(t, key.ID, *apiKeyID)
	})
	t.Run("by_delete_key", func(t *testing.T) {
		_, del := e.deletionKey(t, nil, auth.ScopeDelete)
		require.Equal(t, created.Id, deletionBody(t, e.call(t, http.MethodGet, path, nil, bearer(del)), http.StatusOK).Id)
	})
	t.Run("dashboard_source_without_key", func(t *testing.T) {
		rec := e.call(t, http.MethodPost, deletionsPath(msk),
			map[string]string{"subject_type": "user", "subject_value": "u-dashboard"}, asOwner(ownerCookie(t, e)))
		dr := deletionBody(t, rec, http.StatusAccepted)
		require.Equal(t, gen.DeletionSourceDashboard, dr.Source)
		var apiKeyID *uuid.UUID
		require.NoError(t, e.env.PG.Pool().QueryRow(t.Context(),
			`SELECT api_key_id FROM deletion_requests WHERE id = $1`, dr.Id).Scan(&apiKeyID))
		require.Nil(t, apiKeyID)
	})

	notFound := []struct {
		name   string
		path   string
		secret func(t *testing.T) string
		code   string
	}{
		{"unknown_request", deletionsPath(msk) + "/" + uuid.NewString(), e.newKey, "not_found"},
		{"request_of_other_app", deletionsPath(nyc) + "/" + created.Id.String(), e.newKey, "not_found"},
		{"unknown_app", deletionsPath(uuid.New()) + "/" + created.Id.String(), e.newKey, "app_not_found"},
		{"foreign_app_key", path, func(t *testing.T) string {
			_, s := e.deletionKey(t, &nyc, auth.ScopeRead)
			return s
		}, "app_not_found"},
	}
	for _, tc := range notFound {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, tc.path, nil)
			bearer(tc.secret(t))(req)
			rec := serveRequest(t, e.handler, req)
			requireEnvelope(t, rec, http.StatusNotFound, tc.code)
			validateResponse(t, doc, req, rec)
		})
	}
	t.Run("bad_request_id", func(t *testing.T) {
		rec := e.call(t, http.MethodGet, deletionsPath(msk)+"/not-a-uuid", nil, bearer(e.newKey(t)))
		requireEnvelope(t, rec, http.StatusBadRequest, "invalid_request")
		require.Equal(t, "request_id", errorBody(t, rec, http.StatusBadRequest).Details["field"])
	})
}

func TestListDeletionRequests(t *testing.T) {
	e := newPortfolioEnv(t)
	doc := loadAPIDoc(t)
	msk := testutil.ScenarioApps[0].ID
	nyc := testutil.ScenarioApps[1].ID
	cookie := ownerCookie(t, e)
	_, secret := e.deletionKey(t, nil, auth.ScopeRead, auth.ScopeDelete)

	var ids []uuid.UUID
	for _, tc := range []struct {
		app  uuid.UUID
		user string
	}{{msk, "u1"}, {nyc, "u2"}, {msk, "u3"}} {
		dr := deletionBody(t, e.call(t, http.MethodPost, deletionsPath(tc.app),
			map[string]string{"subject_type": "user", "subject_value": tc.user}, bearer(secret)), http.StatusAccepted)
		ids = append(ids, dr.Id)
	}
	// requested_at — фиксированные часы; порядок задаём явно.
	for i, id := range ids {
		_, err := e.env.PG.Pool().Exec(t.Context(),
			`UPDATE deletion_requests SET requested_at = requested_at + make_interval(mins => $2) WHERE id = $1`, id, i)
		require.NoError(t, err)
	}

	list := func(t *testing.T, query string) []uuid.UUID {
		t.Helper()
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/admin/v1/deletion-requests"+query, nil)
		asOwner(cookie)(req)
		rec := serveRequest(t, e.handler, req)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		validateResponse(t, doc, req, rec)
		var got gen.DeletionRequestList
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
		out := make([]uuid.UUID, 0, len(got.Items))
		for _, it := range got.Items {
			out = append(out, it.Id)
		}
		return out
	}

	t.Run("all_newest_first", func(t *testing.T) {
		require.Equal(t, []uuid.UUID{ids[2], ids[1], ids[0]}, list(t, ""))
	})
	t.Run("filter_by_app", func(t *testing.T) {
		require.Equal(t, []uuid.UUID{ids[2], ids[0]}, list(t, "?app_id="+msk.String()))
		require.Empty(t, list(t, "?app_id="+uuid.NewString()))
	})
	t.Run("bad_app_id", func(t *testing.T) {
		rec := e.call(t, http.MethodGet, "/admin/v1/deletion-requests?app_id=nope", nil, asOwner(cookie))
		requireEnvelope(t, rec, http.StatusBadRequest, "invalid_request")
	})
	t.Run("bearer_is_not_accepted", func(t *testing.T) {
		rec := e.call(t, http.MethodGet, "/admin/v1/deletion-requests", nil, bearer(secret))
		requireEnvelope(t, rec, http.StatusUnauthorized, "unauthenticated")
	})
}
