//go:build integration

package auth_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/api"
	"appmetrica.online/backend/internal/auth"
	"appmetrica.online/backend/internal/testutil"
)

// passkeysAPI — admin API поверх сервиса harness: маршруты /admin/v1/passkeys* с cookie am_session.
type passkeysAPI struct {
	mux   *http.ServeMux
	token string
}

// ownerAPI — owner с одним passkey «MacBook» (аутентификатор a) и его сессия.
func (h *harness) ownerAPI(t *testing.T) (*passkeysAPI, *testutil.Authenticator) {
	t.Helper()
	a := newAuthenticator(t)
	sess, err := h.setup(t, h.newToken(t), a, "MacBook")
	require.NoError(t, err)
	mux := http.NewServeMux()
	api.NewServer(h.svc, nil, slog.New(slog.DiscardHandler), testConfig()).Register(mux)
	return &passkeysAPI{mux: mux, token: sess.Token}, a
}

// call — запрос дашборда: X-Requested-With и cookie сессии (если token не пуст).
func (p *passkeysAPI) call(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return p.callAs(t, method, path, body, p.token)
}

func (p *passkeysAPI) callAs(t *testing.T, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var r *strings.Reader
	switch b := body.(type) {
	case nil:
		r = strings.NewReader("")
	case string:
		r = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		require.NoError(t, err)
		r = strings.NewReader(string(raw))
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Requested-With", "appmetrica-web")
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	}
	rec := httptest.NewRecorder()
	p.mux.ServeHTTP(rec, req)
	return rec
}

type passkeyJSON struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	BackupEligible bool       `json:"backup_eligible"`
	BackupState    bool       `json:"backup_state"`
	Transports     []string   `json:"transports"`
}

func (p *passkeysAPI) list(t *testing.T) []passkeyJSON {
	t.Helper()
	rec := p.call(t, http.MethodGet, "/admin/v1/passkeys", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Items []passkeyJSON `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out.Items
}

func (p *passkeysAPI) begin(t *testing.T) (string, json.RawMessage) {
	t.Helper()
	rec := p.call(t, http.MethodPost, "/admin/v1/passkeys/register/begin", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var c struct {
		CeremonyID string          `json:"ceremony_id"`
		Options    json.RawMessage `json:"options"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &c))
	return c.CeremonyID, c.Options
}

// register проходит register/begin → регистрация на аутентификаторе → register/finish.
func (p *passkeysAPI) register(t *testing.T, a *testutil.Authenticator, name string) *httptest.ResponseRecorder {
	t.Helper()
	id, options := p.begin(t)
	return p.call(t, http.MethodPost, "/admin/v1/passkeys/register/finish", map[string]any{
		"ceremony_id": id, "credential": json.RawMessage(a.Register(t, options)), "name": name,
	})
}

func decodePasskey(t *testing.T, rec *httptest.ResponseRecorder) passkeyJSON {
	t.Helper()
	var pk passkeyJSON
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &pk), rec.Body.String())
	return pk
}

func requireAPIError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) map[string]any {
	t.Helper()
	require.Equal(t, status, rec.Code, rec.Body.String())
	var env struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), rec.Body.String())
	require.Equal(t, code, env.Error.Code)
	require.NotEmpty(t, env.Error.Message)
	return env.Error.Details
}

func (h *harness) passkeyNames(t *testing.T) []string {
	t.Helper()
	rows, err := h.pg.Pool().Query(t.Context(), "SELECT name FROM passkeys ORDER BY created_at, id")
	require.NoError(t, err)
	var names []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		names = append(names, n)
	}
	require.NoError(t, rows.Err())
	return names
}

func TestDeletePasskey_LastForbidden(t *testing.T) {
	h := newHarness(t)
	p, _ := h.ownerAPI(t)
	items := p.list(t)
	require.Len(t, items, 1)

	rec := p.call(t, http.MethodDelete, "/admin/v1/passkeys/"+items[0].ID, nil)
	requireAPIError(t, rec, http.StatusConflict, "last_passkey")
	require.Equal(t, []string{"MacBook"}, h.passkeyNames(t), "passkey на месте")

	err := h.svc.DeletePasskey(t.Context(), uuid.MustParse(items[0].ID))
	require.ErrorIs(t, err, auth.ErrLastPasskey)
	require.Equal(t, 1, h.count(t, "passkeys"))
}

func TestRegisterAdditionalPasskey(t *testing.T) {
	h := newHarness(t)
	p, macbook := h.ownerAPI(t)

	id, options := p.begin(t)
	opts := testutil.ParseCreationOptions(t, options)
	require.Equal(t, []string{macbook.CredentialIDBase64()}, opts.ExcludedIDs(), "excludeCredentials — все текущие passkey")
	require.Equal(t, "required", opts.PublicKey.AuthenticatorSelection.ResidentKey)
	require.Equal(t, "required", opts.PublicKey.AuthenticatorSelection.UserVerification)
	require.Equal(t, testRPID, opts.PublicKey.RP.ID)

	iphone := newAuthenticator(t) // synced passkey: BE и BS, transports hybrid + internal
	rec := p.call(t, http.MethodPost, "/admin/v1/passkeys/register/finish", map[string]any{
		"ceremony_id": id, "credential": json.RawMessage(iphone.Register(t, options)), "name": "iPhone",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	created := decodePasskey(t, rec)
	require.Equal(t, "iPhone", created.Name)
	require.True(t, created.BackupEligible)
	require.True(t, created.BackupState)
	require.Equal(t, []string{"hybrid", "internal"}, created.Transports)
	require.Nil(t, created.LastUsedAt)
	require.False(t, created.CreatedAt.IsZero())
	require.Equal(t, iphone.UserHandle, macbook.UserHandle, "тот же owner")
	require.Equal(t, 0, h.count(t, "webauthn_ceremonies"), "церемония одноразовая")

	yubikey := newAuthenticator(t) // аппаратный ключ: не синхронизируется
	yubikey.BackupEligible, yubikey.BackupState = false, false
	yubikey.Transports = []string{"nfc", "usb"}
	rec = p.register(t, yubikey, "YubiKey")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	items := p.list(t)
	require.Len(t, items, 3)
	require.Equal(t, "MacBook", items[0].Name)
	require.Nil(t, items[0].LastUsedAt, "MacBook ещё ни разу не входил")
	require.Equal(t, created, items[1])
	require.Equal(t, passkeyJSON{
		ID: items[2].ID, Name: "YubiKey", CreatedAt: items[2].CreatedAt,
		BackupEligible: false, BackupState: false, Transports: []string{"nfc", "usb"},
	}, items[2])

	raw := p.call(t, http.MethodGet, "/admin/v1/passkeys", nil).Body.String()
	require.NotContains(t, raw, "credential_id")
	require.NotContains(t, raw, "public_key")

	// Новые passkey годятся для входа.
	for _, a := range []*testutil.Authenticator{iphone, yubikey} {
		_, err := h.login(t, a)
		require.NoError(t, err)
	}
}

func TestRenamePasskey_Validation(t *testing.T) {
	h := newHarness(t)
	p, _ := h.ownerAPI(t)
	id := p.list(t)[0].ID

	for name, value := range map[string]string{
		"empty":       "",
		"spaces only": "   ",
		"65 chars":    strings.Repeat("я", 65),
	} {
		t.Run(name, func(t *testing.T) {
			rec := p.call(t, http.MethodPatch, "/admin/v1/passkeys/"+id, map[string]any{"name": value})
			details := requireAPIError(t, rec, http.StatusBadRequest, "invalid_request")
			fields, ok := details["fields"].(map[string]any)
			require.True(t, ok, details)
			require.Contains(t, fields, "name")
			require.Equal(t, []string{"MacBook"}, h.passkeyNames(t))
		})
	}

	_, err := h.svc.RenamePasskey(t.Context(), uuid.MustParse(id), "")
	var verr *auth.ValidationError
	require.ErrorAs(t, err, &verr)
	require.Equal(t, "name", verr.Field)
}

func TestRenamePasskey(t *testing.T) {
	h := newHarness(t)
	p, _ := h.ownerAPI(t)
	before := p.list(t)[0]

	rec := p.call(t, http.MethodPatch, "/admin/v1/passkeys/"+before.ID, map[string]any{"name": "  MacBook Touch ID "})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	after := decodePasskey(t, rec)
	before.Name = "MacBook Touch ID"
	require.Equal(t, before, after, "меняется только имя, пробелы по краям обрезаны")
	require.Equal(t, []passkeyJSON{after}, p.list(t))

	long := strings.Repeat("я", 64)
	rec = p.call(t, http.MethodPatch, "/admin/v1/passkeys/"+before.ID, map[string]any{"name": long})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Equal(t, long, decodePasskey(t, rec).Name)
}

func TestRenamePasskey_NotFound(t *testing.T) {
	h := newHarness(t)
	p, _ := h.ownerAPI(t)

	rec := p.call(t, http.MethodPatch, "/admin/v1/passkeys/"+uuid.NewString(), map[string]any{"name": "iPhone"})
	requireAPIError(t, rec, http.StatusNotFound, "not_found")
	_, err := h.svc.RenamePasskey(t.Context(), uuid.New(), "iPhone")
	require.ErrorIs(t, err, auth.ErrNotFound)

	rec = p.call(t, http.MethodPatch, "/admin/v1/passkeys/not-a-uuid", map[string]any{"name": "iPhone"})
	requireAPIError(t, rec, http.StatusBadRequest, "invalid_request")
	rec = p.call(t, http.MethodPatch, "/admin/v1/passkeys/"+p.list(t)[0].ID, `{"name":`)
	requireAPIError(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestDeletePasskey_OneOfSeveral(t *testing.T) {
	h := newHarness(t)
	p, macbook := h.ownerAPI(t)
	iphone := newAuthenticator(t)
	require.Equal(t, http.StatusCreated, p.register(t, iphone, "iPhone").Code)
	items := p.list(t)
	require.Len(t, items, 2)

	rec := p.call(t, http.MethodDelete, "/admin/v1/passkeys/"+items[0].ID, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	require.Empty(t, rec.Body.String())
	require.Equal(t, []passkeyJSON{items[1]}, p.list(t))

	_, err := h.login(t, macbook)
	require.ErrorIs(t, err, auth.ErrVerificationFailed, "удалённым passkey не войти")
	_, err = h.login(t, iphone)
	require.NoError(t, err)

	// Повторное удаление и неизвестный id — 404, даже когда остался один passkey.
	requireAPIError(t, p.call(t, http.MethodDelete, "/admin/v1/passkeys/"+items[0].ID, nil), http.StatusNotFound, "not_found")
	requireAPIError(t, p.call(t, http.MethodDelete, "/admin/v1/passkeys/"+uuid.NewString(), nil), http.StatusNotFound, "not_found")
	require.ErrorIs(t, h.svc.DeletePasskey(t.Context(), uuid.New()), auth.ErrNotFound)
	requireAPIError(t, p.call(t, http.MethodDelete, "/admin/v1/passkeys/not-a-uuid", nil), http.StatusBadRequest, "invalid_request")
}

func TestDeletePasskey_ConcurrentLastTwo(t *testing.T) {
	h := newHarness(t)
	p, _ := h.ownerAPI(t)
	require.Equal(t, http.StatusCreated, p.register(t, newAuthenticator(t), "iPhone").Code)
	items := p.list(t)
	require.Len(t, items, 2)

	var (
		wg    sync.WaitGroup
		start = make(chan struct{})
		errs  = make([]error, len(items))
	)
	for i, it := range items {
		wg.Go(func() {
			<-start
			errs[i] = h.svc.DeletePasskey(t.Context(), uuid.MustParse(it.ID))
		})
	}
	close(start)
	wg.Wait()

	var deleted, last int
	for _, err := range errs {
		switch {
		case err == nil:
			deleted++
		case errors.Is(err, auth.ErrLastPasskey):
			last++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, deleted)
	require.Equal(t, 1, last)
	require.Equal(t, 1, h.count(t, "passkeys"), "владелец не остаётся без passkey")
}

func TestFinishPasskeyRegistration_Errors(t *testing.T) {
	h := newHarness(t)
	p, macbook := h.ownerAPI(t)
	finish := func(id string, credential []byte, name string) *httptest.ResponseRecorder {
		return p.call(t, http.MethodPost, "/admin/v1/passkeys/register/finish", map[string]any{
			"ceremony_id": id, "credential": json.RawMessage(credential), "name": name,
		})
	}

	t.Run("duplicate credential", func(t *testing.T) {
		rec := p.register(t, macbook, "MacBook again")
		requireAPIError(t, rec, http.StatusUnprocessableEntity, "webauthn_verification_failed")
		require.Equal(t, []string{"MacBook"}, h.passkeyNames(t))
	})

	t.Run("unknown ceremony", func(t *testing.T) {
		rec := finish(uuid.NewString(), []byte(`{}`), "iPhone")
		requireAPIError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("reused ceremony", func(t *testing.T) {
		id, options := p.begin(t)
		require.Equal(t, http.StatusCreated, finish(id, newAuthenticator(t).Register(t, options), "iPad").Code)
		rec := finish(id, newAuthenticator(t).Register(t, options), "iPad 2")
		requireAPIError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("expired ceremony", func(t *testing.T) {
		id, options := p.begin(t)
		h.clock.Advance(6 * time.Minute)
		rec := finish(id, newAuthenticator(t).Register(t, options), "Pixel")
		requireAPIError(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("setup link ceremony", func(t *testing.T) {
		c, err := h.svc.BeginSetup(t.Context(), h.newToken(t))
		require.NoError(t, err)
		rec := finish(c.ID.String(), newAuthenticator(t).Register(t, c.Options), "Pixel")
		requireAPIError(t, rec, http.StatusUnprocessableEntity, "webauthn_verification_failed")
	})

	t.Run("session ceremony is not a setup ceremony", func(t *testing.T) {
		id, options := p.begin(t)
		token := h.newToken(t)
		_, err := h.svc.FinishSetup(t.Context(), auth.FinishSetupInput{
			Token: token, CeremonyID: uuid.MustParse(id), Credential: newAuthenticator(t).Register(t, options), Name: "Pixel",
		})
		require.ErrorIs(t, err, auth.ErrVerificationFailed)
	})

	t.Run("invalid credential", func(t *testing.T) {
		id, options := p.begin(t)
		a := newAuthenticator(t)
		a.Origin = "https://evil.example"
		requireAPIError(t, finish(id, a.Register(t, options), "Pixel"), http.StatusUnprocessableEntity, "webauthn_verification_failed")

		id, _ = p.begin(t)
		requireAPIError(t, finish(id, []byte(`{"id":"x"}`), "Pixel"), http.StatusUnprocessableEntity, "webauthn_verification_failed")
	})

	t.Run("invalid name", func(t *testing.T) {
		id, options := p.begin(t)
		rec := finish(id, newAuthenticator(t).Register(t, options), strings.Repeat("x", 65))
		details := requireAPIError(t, rec, http.StatusBadRequest, "invalid_request")
		require.Contains(t, details["fields"], "name")
	})

	t.Run("malformed body", func(t *testing.T) {
		rec := p.call(t, http.MethodPost, "/admin/v1/passkeys/register/finish", `{"ceremony_id":`)
		requireAPIError(t, rec, http.StatusBadRequest, "invalid_request")
	})

	require.Equal(t, []string{"MacBook", "iPad"}, h.passkeyNames(t), "passkey добавлен только успешной церемонией")
}

func TestPasskeyEndpoints_RequireSession(t *testing.T) {
	h := newHarness(t)
	p, _ := h.ownerAPI(t)
	id := p.list(t)[0].ID

	routes := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/admin/v1/passkeys", nil},
		{http.MethodPost, "/admin/v1/passkeys/register/begin", nil},
		{http.MethodPost, "/admin/v1/passkeys/register/finish", map[string]any{
			"ceremony_id": uuid.NewString(), "credential": map[string]any{}, "name": "iPhone",
		}},
		{http.MethodPatch, "/admin/v1/passkeys/" + id, map[string]any{"name": "iPhone"}},
		{http.MethodDelete, "/admin/v1/passkeys/" + id, nil},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			requireAPIError(t, p.callAs(t, r.method, r.path, r.body, ""), http.StatusUnauthorized, "unauthenticated")
			requireAPIError(t, p.callAs(t, r.method, r.path, r.body, tamper(p.token)), http.StatusUnauthorized, "unauthenticated")

			// Admin принимает только cookie: Bearer-ключ Read API сюда не пускает.
			req := httptest.NewRequestWithContext(t.Context(), r.method, r.path, strings.NewReader(`{"name":"iPhone"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Requested-With", "appmetrica-web")
			req.Header.Set("Authorization", "Bearer sk_"+strings.Repeat("a", 40))
			rec := httptest.NewRecorder()
			p.mux.ServeHTTP(rec, req)
			requireAPIError(t, rec, http.StatusUnauthorized, "unauthenticated")
		})
	}

	// Изменяющие запросы с валидной cookie без X-Requested-With — 403 csrf_check_failed.
	for _, r := range routes[1:] {
		req := httptest.NewRequestWithContext(t.Context(), r.method, r.path, strings.NewReader(`{"name":"iPhone"}`))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: p.token})
		rec := httptest.NewRecorder()
		p.mux.ServeHTTP(rec, req)
		requireAPIError(t, rec, http.StatusForbidden, "csrf_check_failed")
	}
	require.Equal(t, []string{"MacBook"}, h.passkeyNames(t))

	// После выхода сессия больше не открывает список.
	require.NoError(t, h.svc.Logout(t.Context(), p.token))
	requireAPIError(t, p.call(t, http.MethodGet, "/admin/v1/passkeys", nil), http.StatusUnauthorized, "unauthenticated")
}
