//go:build integration

package reports

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/testutil"
)

// realtimeNow — «сейчас» тестов realtime (realtime.ndjson в 05-data-model.md §11): начало минуты, окно поминутных
// стартов — 11:30…11:59 UTC.
var realtimeNow = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func ago(d time.Duration) string { return realtimeNow.Add(-d).Format(time.RFC3339Nano) }

func realtimeParams(t testing.TB, env string) Params {
	t.Helper()
	msk, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	return Params{AppID: identityApp, Location: msk, Env: env}
}

func realtimeAt(t *testing.T, env *testutil.Env, now time.Time, p Params) Realtime {
	t.Helper()
	got, err := NewService(env.CH.Conn(), clock.NewFixed(now)).Realtime(t.Context(), p)
	require.NoError(t, err)
	require.Len(t, got.PerMinute, RealtimeMinutes)
	require.LessOrEqual(t, len(got.RecentEvents), RecentEventsLimit)
	require.Equal(t, now, got.GeneratedAt)
	return got
}

func sessionsStarted(r Realtime) uint64 {
	var n uint64
	for _, m := range r.PerMinute {
		n += m.SessionsStarted
	}
	return n
}

func feedIDs(r Realtime) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(r.RecentEvents))
	for _, e := range r.RecentEvents {
		out = append(out, e.InsertID)
	}
	return out
}

// received — копия события r, принятого в at (ретрай или поздняя доставка).
func received(r chstore.EventRow, at time.Time) chstore.EventRow {
	r.ReceivedAt = at
	return r
}

// AC-031.1 (AC-5.4.1, тест-вектор active_now): 3 сессии начаты за 30 минут, одна из них завершена, ещё одна
// начата 40 минут назад — active_now = 2, в per_minute 3 старта.
func TestRealtime_ActiveNow(t *testing.T) {
	env := startWith(t, concat(
		visit(t, 1, "", ago(10*time.Minute), 0),
		visit(t, 2, "", ago(5*time.Minute), 4*time.Minute),
		visit(t, 3, "", ago(20*time.Minute), 0),
		visit(t, 4, "", ago(40*time.Minute), 0),
	))

	got := realtimeAt(t, env, realtimeNow, realtimeParams(t, EnvProduction))
	require.Equal(t, uint64(2), got.ActiveNow)
	require.Equal(t, uint64(3), sessionsStarted(got))
}

// active_now — люди (склейка «первый владелец»), а не установки и не сессии; события ретраев, чужого приложения
// и другого environment не учитываются, сессия, начатая раньше окна, не активна даже без session_end.
func TestRealtime_ActiveNowPersons(t *testing.T) {
	otherApp := func(r *chstore.EventRow) { r.AppID = uuid.MustParse("1de0a000-0000-4000-8000-0000000000ff") }
	retried := visit(t, 7, "", ago(3*time.Minute), 0)
	ended := visit(t, 8, "", ago(15*time.Minute), 10*time.Minute)
	env := startWith(t,
		// Установка 1 вчера вошла как U: её анонимная сессия сейчас — человек u:U.
		visit(t, 1, "U", "2026-09-09T10:00:00Z", time.Minute),
		concat(
			visit(t, 1, "", ago(12*time.Minute), 0),
			visit(t, 2, "U", ago(6*time.Minute), 0), // вторая установка U
			visit(t, 3, "V", ago(25*time.Minute), 0),
			visit(t, 3, "V", ago(2*time.Minute), 0), // вторая открытая сессия V
			visit(t, 4, "", ago(31*time.Minute), 0), // начата до окна
			// session_end за окном 30 минут, начало — раньше: сессия не считается начатой в окне.
			visit(t, 5, "", ago(45*time.Minute), 40*time.Minute)[1:],
			with(visit(t, 6, "", ago(4*time.Minute), 0), otherApp),
			[]chstore.EventRow{dev(session(t, 9, "", ago(time.Minute)))},
			retried,
			ended,
		),
		// Ретраи: session_start установки 7 и session_end установки 8 записаны повторно.
		[]chstore.EventRow{received(retried[0], realtimeNow.Add(-time.Minute)), received(ended[1], realtimeNow.Add(-time.Minute))},
	)

	got := realtimeAt(t, env, realtimeNow, realtimeParams(t, EnvProduction))
	require.Equal(t, uint64(3), got.ActiveNow, "u:U, u:V и i:7")
	require.Equal(t, uint64(6), sessionsStarted(got), "сессии установок 1, 2, 3 (две), 7, 8; ретрай — один старт")
}

// Тест-вектор per_minute_len: ровно 30 точек — 30 последних полных минут, текущая неполная минута не входит;
// пустые минуты — 0, начало минуты — в TZ приложения.
func TestRealtime_PerMinuteAlways30(t *testing.T) {
	now := realtimeNow.Add(30 * time.Second) // 12:00:30, текущая минута 12:00 неполная

	t.Run("empty", func(t *testing.T) {
		env := startWith(t)
		got := realtimeAt(t, env, now, realtimeParams(t, EnvProduction))
		require.Zero(t, got.ActiveNow)
		require.Empty(t, got.RecentEvents)
		for i, m := range got.PerMinute {
			require.Zero(t, m.SessionsStarted)
			want := realtimeNow.Add(time.Duration(i-RealtimeMinutes) * time.Minute)
			require.True(t, want.Equal(m.Minute), "минута %d: %s", i, m.Minute)
		}
		require.Equal(t, "2026-09-10T14:30:00+03:00", got.PerMinute[0].Minute.Format(time.RFC3339))
		require.Equal(t, "2026-09-10T14:59:00+03:00", got.PerMinute[RealtimeMinutes-1].Minute.Format(time.RFC3339))
	})

	t.Run("buckets", func(t *testing.T) {
		dup := session(t, 5, "", ago(15*time.Minute-10*time.Second)) // 11:45:10
		env := startWith(t, concat(
			[]chstore.EventRow{session(t, 1, "", ago(30*time.Minute))},                  // 11:30:00 — первая минута
			[]chstore.EventRow{session(t, 2, "", ago(30*time.Minute+time.Millisecond))}, // до окна
			[]chstore.EventRow{session(t, 3, "", ago(-10*time.Second))},                 // текущая минута
			visit(t, 4, "", ago(15*time.Minute-40*time.Second), 10*time.Second),         // 11:45:40, session_end не старт
			[]chstore.EventRow{dup, received(dup, realtimeNow)},                         // ретрай
			[]chstore.EventRow{session(t, 6, "", ago(time.Minute-59*time.Second))},      // 11:59:59.000
		))

		got := realtimeAt(t, env, now, realtimeParams(t, EnvProduction))
		byMinute := map[string]uint64{}
		for _, m := range got.PerMinute {
			byMinute[m.Minute.UTC().Format("15:04")] = m.SessionsStarted
		}
		require.Equal(t, uint64(1), byMinute["11:30"])
		require.Equal(t, uint64(2), byMinute["11:45"], "установки 4 и 5, ретрай установки 5 — одна сессия")
		require.Equal(t, uint64(1), byMinute["11:59"])
		require.Equal(t, uint64(4), sessionsStarted(got))
	})
}

// Тест-вектор recent_limit: из 80 принятых событий в ленте 50 последних по received_at DESC.
func TestRealtime_RecentEventsLimit50(t *testing.T) {
	rows := make([]chstore.EventRow, 0, 80)
	for i := range 80 {
		r := session(t, i+1, "", ago(20*time.Minute))
		r.ReceivedAt = realtimeNow.Add(-time.Duration(80-i) * time.Second)
		rows = append(rows, r)
	}
	env := startWith(t, rows)

	got := realtimeAt(t, env, realtimeNow, realtimeParams(t, EnvProduction))
	require.Len(t, got.RecentEvents, RecentEventsLimit)
	for i, e := range got.RecentEvents {
		want := rows[79-i]
		require.Equal(t, want.InsertID, e.InsertID, "позиция %d", i)
		require.True(t, want.ReceivedAt.Equal(e.ReceivedAt), "позиция %d", i)
	}
}

// Тест-вектор env_filter: при environment=development в срезе только события development, и наоборот.
func TestRealtime_EnvironmentFilter(t *testing.T) {
	prod := visit(t, 1, "", ago(5*time.Minute), 0)
	devRows := with(concat(visit(t, 2, "", ago(6*time.Minute), 0), visit(t, 3, "", ago(7*time.Minute), time.Minute)), func(r *chstore.EventRow) {
		r.Environment = EnvDevelopment
	})
	env := startWith(t, concat(prod, devRows))

	got := realtimeAt(t, env, realtimeNow, realtimeParams(t, EnvDevelopment))
	require.Equal(t, uint64(1), got.ActiveNow)
	require.Equal(t, uint64(2), sessionsStarted(got))
	require.ElementsMatch(t, []uuid.UUID{devRows[0].InsertID, devRows[1].InsertID, devRows[2].InsertID}, feedIDs(got))
	for _, e := range got.RecentEvents {
		require.Equal(t, EnvDevelopment, e.Environment)
	}

	got = realtimeAt(t, env, realtimeNow, realtimeParams(t, ""))
	require.Equal(t, uint64(1), got.ActiveNow, "environment по умолчанию — production")
	require.Equal(t, []uuid.UUID{prod[0].InsertID}, feedIDs(got))
}

// Переключатель платформ (FR-5 multi-platform): фильтр platform сужает active_now, поминутные старты и ленту;
// пустой фильтр — все платформы.
func TestRealtime_PlatformFilter(t *testing.T) {
	android := func(r *chstore.EventRow) { r.Platform = "android" }
	ios := concat(visit(t, 1, "", ago(5*time.Minute), 0), visit(t, 2, "", ago(9*time.Minute), 0))
	droid := with(concat(visit(t, 3, "", ago(6*time.Minute), 0), visit(t, 4, "", ago(8*time.Minute), time.Minute)), android)
	env := startWith(t, concat(ios, droid))

	p := realtimeParams(t, EnvProduction)
	p.Filters.Platform = []string{"android"}
	got := realtimeAt(t, env, realtimeNow, p)
	require.Equal(t, uint64(1), got.ActiveNow, "установка 4 завершила сессию")
	require.Equal(t, uint64(2), sessionsStarted(got))
	require.ElementsMatch(t, []uuid.UUID{droid[0].InsertID, droid[1].InsertID, droid[2].InsertID}, feedIDs(got))

	p.Filters.Platform = []string{"web"}
	got = realtimeAt(t, env, realtimeNow, p)
	require.Zero(t, got.ActiveNow)
	require.Zero(t, sessionsStarted(got))
	require.Empty(t, got.RecentEvents)

	got = realtimeAt(t, env, realtimeNow, realtimeParams(t, EnvProduction))
	require.Equal(t, uint64(3), got.ActiveNow)
	require.Equal(t, uint64(4), sessionsStarted(got))
	require.Len(t, got.RecentEvents, 5)
}

// AC-031.3 (AC-5.4.3): событие, записанное дважды до слияния частей (ретрай с более поздним received_at), в ленте
// одной строкой — копией последнего приёма, как после слияния ReplacingMergeTree(received_at).
func TestRealtime_FeedDeduplicated(t *testing.T) {
	first := session(t, 1, "", ago(10*time.Minute))
	other := session(t, 2, "", ago(8*time.Minute))
	retry := received(first, realtimeNow.Add(-5*time.Minute))
	env := startWith(t, []chstore.EventRow{first, other}, []chstore.EventRow{retry, received(first, first.ReceivedAt)})

	got := realtimeAt(t, env, realtimeNow, realtimeParams(t, EnvProduction))
	require.Equal(t, []uuid.UUID{first.InsertID, other.InsertID}, feedIDs(got))
	require.True(t, retry.ReceivedAt.Equal(got.RecentEvents[0].ReceivedAt))
	require.Equal(t, uint64(2), got.ActiveNow)
	require.Equal(t, uint64(2), sessionsStarted(got))
}

// Строка ленты: поля события и контекста, person_key по склейке; события, принятые раньше 30 минут назад,
// после «сейчас» и других приложений, в ленту не попадают, давние по event_time (поздняя доставка) — попадают.
func TestRealtime_FeedRows(t *testing.T) {
	login := visit(t, 1, "U", "2026-09-09T10:00:00Z", time.Minute)
	anon := session(t, 1, "", ago(3*time.Minute))
	anon.Platform, anon.AppVersion, anon.OSName, anon.OSVersion = "ios", "3.4.0", "iOS", "26.0.1"
	anon.DeviceModel, anon.Country, anon.City = "iPhone17,3", "RU", "Moscow"
	anon.ReceivedAt = anon.EventTime.Add(1500 * time.Millisecond)
	late := session(t, 2, "", "2026-09-05T08:00:00Z")
	late.ReceivedAt = realtimeNow.Add(-20 * time.Minute)
	stale := session(t, 3, "", ago(40*time.Minute))
	stale.ReceivedAt = realtimeNow.Add(-30*time.Minute - time.Millisecond)
	future := session(t, 4, "", ago(time.Minute))
	future.ReceivedAt = realtimeNow.Add(time.Second)
	foreign := session(t, 5, "", ago(time.Minute))
	foreign.AppID = uuid.MustParse("1de0a000-0000-4000-8000-0000000000ff")
	env := startWith(t, login, []chstore.EventRow{anon, late, stale, future, foreign})

	got := realtimeAt(t, env, realtimeNow, realtimeParams(t, EnvProduction))
	require.Equal(t, []uuid.UUID{anon.InsertID, late.InsertID}, feedIDs(got))
	e := got.RecentEvents[0]
	require.True(t, anon.EventTime.Equal(e.EventTime))
	require.True(t, anon.ReceivedAt.Equal(e.ReceivedAt))
	require.Equal(t, RecentEvent{
		InsertID: anon.InsertID, EventTime: e.EventTime, ReceivedAt: e.ReceivedAt,
		Name: "session_start", Environment: EnvProduction, Platform: "ios", AppVersion: "3.4.0",
		OSName: "iOS", OSVersion: "26.0.1", DeviceModel: "iPhone17,3", Country: "RU", City: "Moscow",
		InstallID: install(1), PersonKey: "u:U",
	}, e)
	require.Equal(t, installKey(2), got.RecentEvents[1].PersonKey)
	require.True(t, late.EventTime.Equal(got.RecentEvents[1].EventTime))
}
