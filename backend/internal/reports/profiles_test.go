//go:build integration

package reports

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/testutil"
)

var profileNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func profileService(env *testutil.Env) *Service {
	return NewService(env.CH.Conn(), clock.NewFixed(profileNow))
}

func searchProfiles(t *testing.T, svc *Service, q string) []ProfileSummary {
	t.Helper()
	got, err := svc.SearchProfiles(t.Context(), identityApp, q)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.LessOrEqual(t, len(got), MaxProfileResults)
	return got
}

func profileOf(t *testing.T, svc *Service, q ProfileQuery) Profile {
	t.Helper()
	got, err := svc.Profile(t.Context(), identityApp, q)
	require.NoError(t, err)
	require.NotNil(t, got.Installs)
	require.NotNil(t, got.Sessions)
	return got
}

func sessionIDsOf(p Profile) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(p.Sessions))
	for _, s := range p.Sessions {
		out = append(out, s.SessionID)
	}
	return out
}

func installIDsOf(p Profile) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(p.Installs))
	for _, i := range p.Installs {
		out = append(out, i.InstallID)
	}
	return out
}

func ts(t testing.TB, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	require.NoError(t, err)
	return v.UTC()
}

// sessionsEvery — n сессий установок installs по кругу пользователя user, начиная с from, каждая через step,
// длительностью минута.
func sessionsEvery(t testing.TB, n int, user string, from time.Time, step time.Duration, installs ...int) []chstore.EventRow {
	t.Helper()
	var out []chstore.EventRow
	for i := range n {
		at := from.Add(time.Duration(i) * step)
		out = append(out, visit(t, installs[i%len(installs)], user, at.Format(time.RFC3339Nano), time.Minute)...)
	}
	return out
}

// profilesScenario — установка 1: анонимна до входа U, затем U; установка 2 — U; установка 3 — анонимная.
func profilesScenario(t *testing.T) []chstore.EventRow {
	t.Helper()
	return concat(
		visit(t, 1, "", "2026-09-01T07:12:00Z", time.Minute),
		visit(t, 1, "U", "2026-09-02T08:00:00Z", time.Minute),
		visit(t, 2, "U", "2026-10-13T09:11:58.004Z", 3*time.Minute+3*time.Second+986*time.Millisecond),
		visit(t, 3, "", "2026-10-01T10:00:00Z", 0),
	)
}

// AC-032.1 (AC-5.5.1, тест-вектор search_install_linked): установка, привязанная к пользователю, находится как
// его профиль u:U с matched_by=install_id; first/last seen и число установок — человека.
func TestSearchProfiles_InstallResolvesToPerson(t *testing.T) {
	svc := profileService(startWith(t, profilesScenario(t)))

	for _, q := range []string{install(1).String(), strings.ToUpper(install(1).String())} {
		got := searchProfiles(t, svc, q)
		require.Len(t, got, 1, "q=%s", q)
		require.Equal(t, "u:U", got[0].PersonKey.String())
		require.Equal(t, MatchedByInstallID, got[0].MatchedBy)
		require.Equal(t, 2, got[0].InstallsCount)
		require.Equal(t, ts(t, "2026-09-01T07:12:00Z"), got[0].FirstSeen.UTC())
		require.Equal(t, ts(t, "2026-10-13T09:15:01.990Z"), got[0].LastSeen.UTC())
	}
}

// Тест-вектор search_user: поиск по user_id, в том числе пользователя, известного только в development.
func TestSearch_ByUserID(t *testing.T) {
	svc := profileService(startWith(t, concat(
		profilesScenario(t),
		with(visit(t, 4, "D", "2026-10-02T10:00:00Z", time.Minute), func(r *chstore.EventRow) { r.Environment = EnvDevelopment }),
	)))

	got := searchProfiles(t, svc, "U")
	require.Len(t, got, 1)
	require.Equal(t, identity.UserKey("U"), got[0].PersonKey)
	require.Equal(t, MatchedByUserID, got[0].MatchedBy)
	require.Equal(t, 2, got[0].InstallsCount)
	require.Equal(t, ts(t, "2026-09-01T07:12:00Z"), got[0].FirstSeen.UTC(), "анонимные события до входа — тоже U")

	got = searchProfiles(t, svc, "D")
	require.Len(t, got, 1)
	require.Equal(t, identity.UserKey("D"), got[0].PersonKey)
	require.Equal(t, 1, got[0].InstallsCount)

	require.Empty(t, searchProfiles(t, svc, "u"), "точное совпадение с учётом регистра")
	require.Empty(t, searchProfiles(t, svc, "U "), "без обрезки пробелов")
}

// Тест-вектор search_anonymous: анонимная установка — профиль i:<install> без user_id.
func TestSearch_AnonymousInstall(t *testing.T) {
	svc := profileService(startWith(t, profilesScenario(t)))

	got := searchProfiles(t, svc, install(3).String())
	require.Len(t, got, 1)
	require.Equal(t, identity.InstallKey(install(3)), got[0].PersonKey)
	require.False(t, got[0].PersonKey.IsUser())
	require.Equal(t, MatchedByInstallID, got[0].MatchedBy)
	require.Equal(t, 1, got[0].InstallsCount)
	require.Equal(t, ts(t, "2026-10-01T10:00:00Z"), got[0].FirstSeen.UTC())
	require.Equal(t, ts(t, "2026-10-01T10:00:00Z"), got[0].LastSeen.UTC())
}

// q в виде UUID ищется и как install_id, и как user_id: оба совпадения — отдельные профили.
func TestSearch_UUIDAsInstallAndUser(t *testing.T) {
	svc := profileService(startWith(t, concat(
		visit(t, 4, "", "2026-10-01T10:00:00Z", time.Minute),
		visit(t, 5, install(4).String(), "2026-10-02T10:00:00Z", time.Minute),
	)))

	got := searchProfiles(t, svc, install(4).String())
	require.Len(t, got, 2)
	require.Equal(t, identity.InstallKey(install(4)), got[0].PersonKey)
	require.Equal(t, MatchedByInstallID, got[0].MatchedBy)
	require.Equal(t, identity.UserKey(install(4).String()), got[1].PersonKey)
	require.Equal(t, MatchedByUserID, got[1].MatchedBy)
}

// Тест-вектор search_none: нет совпадений — пустой список; установка и пользователь другого приложения не видны.
func TestSearch_NoMatch(t *testing.T) {
	otherApp := func(r *chstore.EventRow) { r.AppID = uuid.MustParse("1de0a000-0000-4000-8000-0000000000ff") }
	svc := profileService(startWith(t, concat(
		profilesScenario(t),
		with(visit(t, 9, "W", "2026-10-01T10:00:00Z", time.Minute), otherApp),
	)))

	for _, q := range []string{"unknown", uuid.NewString(), install(9).String(), "W", "", "u:U", "i:" + install(3).String()} {
		require.Empty(t, searchProfiles(t, svc, q), "q=%q", q)
	}
}

// AC-032.2 (AC-5.5.2): у профиля 150 сессий на 2 установках — страница 100 и страница 50 по курсору без
// пересечений, installs — 2 строки.
func TestGetProfile_PaginationAndInstalls(t *testing.T) {
	rows := sessionsEvery(t, 150, "U", ts(t, "2026-08-01T10:00:00Z"), time.Hour, 1, 2)
	svc := profileService(startWith(t, rows))

	first := profileOf(t, svc, ProfileQuery{PersonKey: "u:U", SessionsLimit: 100})
	require.Len(t, first.Sessions, 100)
	require.NotNil(t, first.NextBefore)
	require.ElementsMatch(t, []uuid.UUID{install(1), install(2)}, installIDsOf(first))

	second := profileOf(t, svc, ProfileQuery{PersonKey: "u:U", SessionsLimit: 100, Before: *first.NextBefore})
	require.Len(t, second.Sessions, 50)
	require.Nil(t, second.NextBefore)
	require.Len(t, second.Installs, 2)

	seen := map[uuid.UUID]bool{}
	for _, id := range append(sessionIDsOf(first), sessionIDsOf(second)...) {
		require.False(t, seen[id], "сессия %s на двух страницах", id)
		seen[id] = true
	}
	require.Len(t, seen, 150)
	for _, r := range rows {
		require.True(t, seen[r.SessionID])
	}
}

// Тест-вектор profile_pagination: сессии по started_at DESC, next_before = started_at последней сессии
// страницы, если есть ещё; before — исключающая граница с точностью до миллисекунды.
func TestProfile_CursorPagination(t *testing.T) {
	rows := sessionsEvery(t, 4, "", ts(t, "2026-10-01T10:00:00.250Z"), time.Hour, 3)
	svc := profileService(startWith(t, rows))
	key := installKey(3)
	starts := func(p Profile) []time.Time {
		out := make([]time.Time, 0, len(p.Sessions))
		for _, s := range p.Sessions {
			out = append(out, s.StartedAt.UTC())
		}
		return out
	}
	at := func(i int) time.Time { return rows[2*i].EventTime }

	page := profileOf(t, svc, ProfileQuery{PersonKey: key, SessionsLimit: 2})
	require.Equal(t, []time.Time{at(3), at(2)}, starts(page))
	require.NotNil(t, page.NextBefore)
	require.Equal(t, at(2), page.NextBefore.UTC())

	page = profileOf(t, svc, ProfileQuery{PersonKey: key, SessionsLimit: 2, Before: *page.NextBefore})
	require.Equal(t, []time.Time{at(1), at(0)}, starts(page))
	require.Nil(t, page.NextBefore, "ровно limit оставшихся сессий — страниц больше нет")

	page = profileOf(t, svc, ProfileQuery{PersonKey: key, SessionsLimit: 2, Before: at(0)})
	require.Empty(t, page.Sessions)
	require.Nil(t, page.NextBefore)
	require.Len(t, page.Installs, 1, "установки и first/last seen не зависят от страницы")
	require.Equal(t, at(0), page.FirstSeen.UTC())

	page = profileOf(t, svc, ProfileQuery{PersonKey: key, Before: at(1).Add(500 * time.Microsecond)})
	require.Equal(t, []time.Time{at(1), at(0)}, starts(page), "курсор точнее миллисекунды не теряет сессию at(1)")

	page = profileOf(t, svc, ProfileQuery{PersonKey: key})
	require.Len(t, page.Sessions, 4, "sessions_limit по умолчанию — 100")
	require.Nil(t, page.NextBefore)
}

// Тест-вектор open_session: у сессии без session_end ended_at и duration_ms — nil; у завершённой — время
// session_end и его duration_ms; сессия без session_start начинается с первого события.
func TestProfile_OpenSession(t *testing.T) {
	closed := visit(t, 3, "", "2026-10-01T10:00:00Z", 90*time.Second)
	open := visit(t, 3, "", "2026-10-02T10:00:00Z", 0)
	orphanEnd := visit(t, 3, "", "2026-10-03T10:00:00Z", time.Minute)[1:]
	svc := profileService(startWith(t, concat(closed, open, orphanEnd)))

	got := profileOf(t, svc, ProfileQuery{PersonKey: installKey(3)})
	require.Len(t, got.Sessions, 3)
	byID := map[uuid.UUID]ProfileSession{}
	for _, s := range got.Sessions {
		byID[s.SessionID] = s
	}

	s := byID[open[0].SessionID]
	require.Nil(t, s.EndedAt)
	require.Nil(t, s.DurationMs)
	require.Equal(t, open[0].EventTime, s.StartedAt.UTC())

	s = byID[closed[0].SessionID]
	require.NotNil(t, s.EndedAt)
	require.Equal(t, closed[1].EventTime, s.EndedAt.UTC())
	require.NotNil(t, s.DurationMs)
	require.Equal(t, uint32(90_000), *s.DurationMs)

	s = byID[orphanEnd[0].SessionID]
	require.Equal(t, orphanEnd[0].EventTime, s.StartedAt.UTC())
	require.NotNil(t, s.DurationMs)
	require.Equal(t, uint32(60_000), *s.DurationMs)
}

// Карточка u:U: установки с контекстом из последнего события (по event_time), установки, где U входил после
// другого первого владельца; сессии всех environment; анонимные сессии чужой установки и ретраи не попадают.
func TestProfile_InstallsAndSessions(t *testing.T) {
	ctxAt := func(rows []chstore.EventRow, appVersion, osVersion, country, city string) []chstore.EventRow {
		return with(rows, func(r *chstore.EventRow) {
			r.OSName, r.OSVersion, r.AppVersion = "iOS", osVersion, appVersion
			r.DeviceModel, r.DeviceType, r.Locale = "iPhone17,3", "phone", "ru-RU"
			r.Country, r.City = country, city
		})
	}
	older := ctxAt(visit(t, 1, "U", "2026-09-01T07:12:00Z", time.Minute), "3.3.0", "26.0", "RU", "Moscow")
	newer := ctxAt(visit(t, 1, "U", "2026-10-10T07:00:00Z", time.Minute), "3.4.0", "26.0.1", "RU", "Kazan")
	// Ретрай session_start принят позже, но event_time тот же: атрибуты установки — из последнего события.
	retry := received(older[0], ts(t, "2026-10-12T00:00:00Z"))
	devSession := with(visit(t, 2, "U", "2026-10-11T07:00:00Z", 0), func(r *chstore.EventRow) {
		r.Environment, r.Platform = EnvDevelopment, "android"
	})
	// Установка 3: первый владелец V, потом в ней входил U — её анонимные сессии принадлежат V.
	foreignAnon := visit(t, 3, "", "2026-10-05T10:00:00Z", time.Minute)
	foreign := concat(
		visit(t, 3, "V", "2026-09-15T10:00:00Z", time.Minute),
		foreignAnon,
		visit(t, 3, "U", "2026-10-06T10:00:00Z", time.Minute),
	)
	svc := profileService(startWith(t, concat(older, newer, devSession, foreign), []chstore.EventRow{retry}))

	got := profileOf(t, svc, ProfileQuery{PersonKey: "u:U"})
	require.Equal(t, identity.UserKey("U"), got.PersonKey)
	require.Equal(t, ts(t, "2026-09-01T07:12:00Z"), got.FirstSeen.UTC())
	require.Equal(t, ts(t, "2026-10-11T07:00:00Z"), got.LastSeen.UTC())
	require.Equal(t, []uuid.UUID{install(2), install(1), install(3)}, installIDsOf(got), "установки по last_seen DESC")

	inst := got.Installs[1]
	require.Equal(t, ProfileInstall{
		InstallID: install(1), Platform: "ios", OSName: "iOS", OSVersion: "26.0.1", AppVersion: "3.4.0",
		DeviceModel: "iPhone17,3", DeviceType: "phone", Locale: "ru-RU", Country: "RU", City: "Kazan",
		FirstSeen: inst.FirstSeen, LastSeen: inst.LastSeen,
	}, inst)
	require.Equal(t, ts(t, "2026-09-01T07:12:00Z"), inst.FirstSeen.UTC())
	require.Equal(t, ts(t, "2026-10-10T07:01:00Z"), inst.LastSeen.UTC())

	require.Equal(t, []uuid.UUID{devSession[0].SessionID, newer[0].SessionID, foreign[4].SessionID, older[0].SessionID},
		sessionIDsOf(got), "сессии по started_at DESC, ретрай — одна сессия, анонимная сессия V — не U")
	devS := got.Sessions[0]
	require.Equal(t, EnvDevelopment, devS.Environment)
	require.Equal(t, "android", devS.Platform)
	require.Equal(t, install(2), devS.InstallID)
	s := got.Sessions[1]
	require.Equal(t, EnvProduction, s.Environment)
	require.Equal(t, "3.4.0", s.AppVersion)
	require.Equal(t, "RU", s.Country)
	require.Equal(t, "Kazan", s.City)

	v := profileOf(t, svc, ProfileQuery{PersonKey: "u:V"})
	require.Equal(t, []uuid.UUID{foreignAnon[0].SessionID, foreign[0].SessionID}, sessionIDsOf(v))
	require.Equal(t, []uuid.UUID{install(3)}, installIDsOf(v))
}

// AC-032.3 (AC-5.5.3): неизвестный person_key — ErrProfileNotFound, неверный формат — identity.ErrInvalidPersonKey;
// неверный sessions_limit — ErrUnsupported.
func TestGetProfile_Errors(t *testing.T) {
	otherApp := func(r *chstore.EventRow) { r.AppID = uuid.MustParse("1de0a000-0000-4000-8000-0000000000ff") }
	svc := profileService(startWith(t, concat(
		profilesScenario(t),
		with(visit(t, 9, "W", "2026-10-01T10:00:00Z", time.Minute), otherApp),
	)))

	for _, key := range []string{
		"u:nobody",
		"i:" + uuid.NewString(),
		"u:W",         // пользователь другого приложения
		installKey(9), // установка другого приложения
		installKey(1), // установка склеена с U: её события — профиль u:U
	} {
		_, err := svc.Profile(t.Context(), identityApp, ProfileQuery{PersonKey: key})
		require.ErrorIs(t, err, ErrProfileNotFound, "person_key=%q", key)
	}

	for _, key := range []string{"x:1", "", "U", "u:", "i:not-a-uuid"} {
		_, err := svc.Profile(t.Context(), identityApp, ProfileQuery{PersonKey: key})
		require.ErrorIs(t, err, identity.ErrInvalidPersonKey, "person_key=%q", key)
	}

	for _, limit := range []int{-1, MaxSessionsLimit + 1} {
		_, err := svc.Profile(t.Context(), identityApp, ProfileQuery{PersonKey: "u:U", SessionsLimit: limit})
		require.ErrorIs(t, err, ErrUnsupported, "sessions_limit=%d", limit)
	}
	got := profileOf(t, svc, ProfileQuery{PersonKey: "u:U", SessionsLimit: MaxSessionsLimit})
	require.Len(t, got.Sessions, 3)
}
