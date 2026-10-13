//go:build integration

package apps_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/auth"
)

// listWriteKeys — GET /admin/v1/apps/{app_id}/write-keys, требует 200.
func (e *env) listWriteKeys(t *testing.T, appID string) []writeKeyJSON {
	t.Helper()
	rec := e.call(t, http.MethodGet, "/admin/v1/apps/"+appID+"/write-keys", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	return decode[struct {
		Items []writeKeyJSON `json:"items"`
	}](t, rec).Items
}

// createWriteKey — POST /admin/v1/apps/{app_id}/write-keys, требует 201.
func (e *env) createWriteKey(t *testing.T, appID string, body any) writeKeyJSON {
	t.Helper()
	rec := e.call(t, http.MethodPost, "/admin/v1/apps/"+appID+"/write-keys", body)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	return decode[writeKeyJSON](t, rec)
}

func (e *env) revokeWriteKey(t *testing.T, appID, keyID string) *httptest.ResponseRecorder {
	t.Helper()
	return e.call(t, http.MethodPost, "/admin/v1/apps/"+appID+"/write-keys/"+keyID+"/revoke", nil)
}

func TestWriteKeys_CreateListRevoke(t *testing.T) {
	e := newEnv(t)
	app, first := e.create(t, "Notes iOS", "ios", "Europe/Moscow")

	e.clock.Advance(time.Minute)
	second := e.createWriteKey(t, app.ID, map[string]any{"label": "Android 3.x"})
	require.Regexp(t, writeKeyRe, second.Key)
	require.NotEqual(t, first.Key, second.Key)
	require.Equal(t, app.ID, second.AppID)
	require.Equal(t, "Android 3.x", second.Label)
	require.Nil(t, second.RevokedAt)
	require.Nil(t, second.LastUsedAt)

	// Оба ключа активны одновременно: ingest принимает каждый под своим write_key_id.
	cache := e.keyCache(t)
	for _, k := range []writeKeyJSON{first, second} {
		info, err := cache.Lookup(k.Key)
		require.NoError(t, err)
		require.Equal(t, k.ID, info.WriteKeyID.String())
	}

	e.clock.Advance(time.Minute)
	revokedAt := e.clock.Now()
	rec := e.revokeWriteKey(t, app.ID, first.ID)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	// Список включает отозванные, новые — первыми.
	list := e.listWriteKeys(t, app.ID)
	require.Len(t, list, 2)
	require.Equal(t, second.ID, list[0].ID)
	require.Nil(t, list[0].RevokedAt)
	require.Equal(t, first.ID, list[1].ID)
	require.NotNil(t, list[1].RevokedAt)
	require.True(t, list[1].RevokedAt.Equal(revokedAt), list[1].RevokedAt)

	cache = e.keyCache(t)
	requireIngestError(t, cache, first.Key, "invalid_app_key")
	_, err := cache.Lookup(second.Key)
	require.NoError(t, err)
}

func TestWriteKeys_RevokeIdempotentAndLastKey(t *testing.T) {
	e := newEnv(t)
	app, key := e.create(t, "Notes iOS", "ios", "Europe/Moscow")

	// Отзыв последнего активного ключа разрешён: приём событий останавливается.
	require.Equal(t, http.StatusNoContent, e.revokeWriteKey(t, app.ID, key.ID).Code)
	e.clock.Advance(time.Hour)
	require.Equal(t, http.StatusNoContent, e.revokeWriteKey(t, app.ID, key.ID).Code)

	list := e.listWriteKeys(t, app.ID)
	require.Len(t, list, 1)
	require.NotNil(t, list[0].RevokedAt)
	require.True(t, list[0].RevokedAt.Equal(t0), "повторный отзыв не меняет revoked_at")
	requireIngestError(t, e.keyCache(t), key.Key, "invalid_app_key")
}

func TestWriteKeys_Labels(t *testing.T) {
	e := newEnv(t)
	app, _ := e.create(t, "Notes iOS", "ios", "Europe/Moscow")

	require.Empty(t, e.createWriteKey(t, app.ID, map[string]any{}).Label)
	require.Equal(t, "android-2026", e.createWriteKey(t, app.ID, map[string]any{"label": "  android-2026 "}).Label)
	require.Equal(t, strings.Repeat("ж", 64), e.createWriteKey(t, app.ID, map[string]any{"label": strings.Repeat("ж", 64)}).Label)

	rec := e.call(t, http.MethodPost, "/admin/v1/apps/"+app.ID+"/write-keys", map[string]any{"label": strings.Repeat("ж", 65)})
	requireFieldError(t, rec, "label")
	rec = e.call(t, http.MethodPost, "/admin/v1/apps/"+app.ID+"/write-keys", `{"label":`)
	requireAPIError(t, rec, http.StatusBadRequest, "invalid_request")
	require.Len(t, e.listWriteKeys(t, app.ID), 4)
}

func TestWriteKeys_NotFound(t *testing.T) {
	e := newEnv(t)
	app, key := e.create(t, "Notes iOS", "ios", "Europe/Moscow")
	other, otherKey := e.create(t, "Notes Android", "android", "UTC")
	unknown := uuid.NewString()

	requireAPIError(t, e.call(t, http.MethodGet, "/admin/v1/apps/"+unknown+"/write-keys", nil), http.StatusNotFound, "app_not_found")
	requireAPIError(t, e.call(t, http.MethodPost, "/admin/v1/apps/"+unknown+"/write-keys", map[string]any{}), http.StatusNotFound, "app_not_found")
	requireAPIError(t, e.revokeWriteKey(t, unknown, key.ID), http.StatusNotFound, "app_not_found")
	requireAPIError(t, e.revokeWriteKey(t, app.ID, unknown), http.StatusNotFound, "not_found")
	// Ключ другого приложения через чужой app_id не отзывается.
	requireAPIError(t, e.revokeWriteKey(t, app.ID, otherKey.ID), http.StatusNotFound, "not_found")
	requireAPIError(t, e.revokeWriteKey(t, app.ID, "not-a-uuid"), http.StatusBadRequest, "invalid_request")

	require.Nil(t, e.listWriteKeys(t, other.ID)[0].RevokedAt)
	require.Nil(t, e.listWriteKeys(t, app.ID)[0].RevokedAt)
}

func TestWriteKeys_RequireSessionAndCSRF(t *testing.T) {
	e := newEnv(t)
	app, key := e.create(t, "Notes iOS", "ios", "Europe/Moscow")
	anonymous := func(r *http.Request) { r.Header.Set("X-Requested-With", auth.RequestedWith) }
	noCSRF := func(r *http.Request) { r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: e.token}) }
	base := "/admin/v1/apps/" + app.ID + "/write-keys"

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, base},
		{http.MethodPost, base},
		{http.MethodPost, base + "/" + key.ID + "/revoke"},
	} {
		requireAPIError(t, e.callWith(t, tc.method, tc.path, map[string]any{}, anonymous), http.StatusUnauthorized, "unauthenticated")
		if tc.method != http.MethodGet {
			requireAPIError(t, e.callWith(t, tc.method, tc.path, map[string]any{}, noCSRF), http.StatusForbidden, "csrf_check_failed")
		}
	}
	list := e.listWriteKeys(t, app.ID)
	require.Len(t, list, 1)
	require.Nil(t, list[0].RevokedAt)
}
