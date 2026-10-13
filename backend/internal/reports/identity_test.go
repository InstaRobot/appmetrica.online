//go:build integration

package reports

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/testutil"
)

// Запросы проверки склейки собраны из канонических фрагментов (identity.LinksCTE, identity.PersonsFirstCTE,
// evCTE), как их собирают отчёты 05-data-model.md §10.4.
const (
	// qPersonDays — люди периода по локальным дням приложения.
	qPersonDays = `WITH ` + identity.LinksCTE + `, ` + evCTE + `
SELECT DISTINCT toString(toDate(event_time, {tz:String})) AS day, person_key FROM ev ORDER BY day, person_key`

	qActive = `WITH ` + identity.LinksCTE + `, ` + evCTE + `
SELECT uniqExact(person_key) AS active FROM ev`

	// qNew — активные в периоде люди, чей first_seen попадает в период (summary.new).
	qNew = `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `, ` + evCTE + `
SELECT uniqExact(a.person_key) AS new
FROM ev AS a INNER JOIN persons_first AS pf ON pf.person_key = a.person_key
WHERE pf.first_seen >= {start_utc:DateTime64(3)} AND pf.first_seen < {end_utc:DateTime64(3)}`

	// qDaily — DAU и новые по локальным дням (определения таблицы README фикстур).
	qDaily = `WITH ` + identity.LinksCTE + `, ` + identity.PersonsFirstCTE + `, ` + evCTE + `,
pd AS (SELECT DISTINCT toDate(event_time, {tz:String}) AS day, person_key FROM ev)
SELECT toString(pd.day) AS day, uniqExact(pd.person_key) AS active,
       uniqExactIf(pd.person_key, toDate(pf.first_seen, {tz:String}) = pd.day) AS new
FROM pd INNER JOIN persons_first AS pf ON pf.person_key = pd.person_key
GROUP BY pd.day ORDER BY pd.day`
)

var identityApp = uuid.MustParse("1de0a000-0000-4000-8000-000000000001")

func install(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("1de10000-0000-4000-8000-%012d", n))
}

func installKey(n int) string { return identity.InstallKey(install(n)).String() }

// session — session_start установки n приложения identityApp; user "" — анонимное событие, at — RFC 3339.
func session(t testing.TB, n int, user, at string) chstore.EventRow {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, at)
	require.NoError(t, err)
	ts = ts.UTC()
	return chstore.EventRow{
		AppID:       identityApp,
		EventDate:   time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, time.UTC),
		EventTime:   ts,
		ClientTime:  ts,
		SentAt:      ts,
		ReceivedAt:  ts,
		InsertID:    uuid.New(),
		Name:        "session_start",
		InstallID:   install(n),
		UserID:      user,
		SessionID:   uuid.New(),
		Environment: EnvProduction,
		Platform:    "ios",
	}
}

func dev(r chstore.EventRow) chstore.EventRow {
	r.Environment = EnvDevelopment
	return r
}

// startWith поднимает изолированные базы теста и вставляет батчи событий по порядку (каждый — отдельной вставкой).
func startWith(t *testing.T, batches ...[]chstore.EventRow) *testutil.Env {
	t.Helper()
	env := testutil.Start(t)
	for _, b := range batches {
		require.NoError(t, env.CH.InsertEvents(t.Context(), b))
	}
	return env
}

// period — параметры отчёта identityApp в Europe/Moscow за from..to (production, без фильтров).
func period(t testing.TB, unit identity.Unit, from, to string) Params {
	t.Helper()
	msk, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	return Params{
		AppID:    identityApp,
		Location: msk,
		Env:      EnvProduction,
		Unit:     unit,
		Period:   Period{From: date(t, from), To: date(t, to)},
	}
}

func personDays(t *testing.T, env *testutil.Env, p Params) map[string][]string {
	t.Helper()
	rows, err := env.CH.Conn().Query(p.Bind(t.Context()), qPersonDays)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	out := map[string][]string{}
	for rows.Next() {
		var day, key string
		require.NoError(t, rows.Scan(&day, &key))
		out[day] = append(out[day], key)
	}
	require.NoError(t, rows.Err())
	return out
}

func count(t *testing.T, env *testutil.Env, p Params, query string) uint64 {
	t.Helper()
	var n uint64
	require.NoError(t, env.CH.Conn().QueryRow(p.Bind(t.Context()), query).Scan(&n))
	return n
}

// AC-027.1: анонимные дни установки до входа приписываются пользователю ретроактивно.
func TestPersonKey_AnonymousDaysMergedIntoFirstOwner(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "", "2026-10-01T12:00:00+03:00"),
		session(t, 1, "", "2026-10-02T12:00:00+03:00"),
		session(t, 1, "", "2026-10-03T12:00:00+03:00"),
		session(t, 1, "u-1", "2026-10-04T12:00:00+03:00"),
	})

	p := period(t, identity.Persons, "2026-10-01", "2026-10-04")
	require.Equal(t, map[string][]string{
		"2026-10-01": {"u:u-1"},
		"2026-10-02": {"u:u-1"},
		"2026-10-03": {"u:u-1"},
		"2026-10-04": {"u:u-1"},
	}, personDays(t, env, p))
	require.Equal(t, uint64(1), count(t, env, p, qActive))
	require.Equal(t, uint64(1), count(t, env, p, qNew), "человек новый в периоде")

	// first_seen человека — первое анонимное событие установки, а не вход.
	require.Equal(t, uint64(1), count(t, env, period(t, identity.Persons, "2026-10-01", "2026-10-01"), qNew))
	require.Equal(t, uint64(0), count(t, env, period(t, identity.Persons, "2026-10-04", "2026-10-04"), qNew))

	inst := period(t, identity.Installs, "2026-10-01", "2026-10-04")
	require.Equal(t, map[string][]string{
		"2026-10-01": {installKey(1)},
		"2026-10-02": {installKey(1)},
		"2026-10-03": {installKey(1)},
		"2026-10-04": {installKey(1)},
	}, personDays(t, env, inst))
}

// AC-027.2: общий девайс — анонимные сессии принадлежат первому владельцу, явные — своему user_id.
func TestPersonKey_FirstOwnerWinsForAnonymous(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "", "2026-10-01T12:00:00+03:00"),
		session(t, 1, "A", "2026-10-02T12:00:00+03:00"),
		session(t, 1, "", "2026-10-03T12:00:00+03:00"), // логаут
		session(t, 1, "B", "2026-10-04T12:00:00+03:00"),
		session(t, 1, "", "2026-10-05T12:00:00+03:00"),
	})

	p := period(t, identity.Persons, "2026-10-01", "2026-10-05")
	require.Equal(t, map[string][]string{
		"2026-10-01": {"u:A"},
		"2026-10-02": {"u:A"},
		"2026-10-03": {"u:A"},
		"2026-10-04": {"u:B"},
		"2026-10-05": {"u:A"},
	}, personDays(t, env, p))
	require.Equal(t, uint64(2), count(t, env, p, qActive))
	require.Equal(t, uint64(1), count(t, env, period(t, identity.Installs, "2026-10-01", "2026-10-05"), qActive))
}

// Первый владелец — по event_time, а не по порядку прихода: досланная позже история A старше входа B.
func TestPersonKey_FirstOwnerByEventTimeNotArrival(t *testing.T) {
	env := startWith(t,
		[]chstore.EventRow{
			session(t, 1, "B", "2026-10-04T12:00:00+03:00"),
			session(t, 1, "", "2026-10-05T12:00:00+03:00"),
		},
		[]chstore.EventRow{session(t, 1, "A", "2026-10-02T12:00:00+03:00")},
	)

	require.Equal(t, map[string][]string{
		"2026-10-02": {"u:A"},
		"2026-10-04": {"u:B"},
		"2026-10-05": {"u:A"},
	}, personDays(t, env, period(t, identity.Persons, "2026-10-01", "2026-10-05")))
}

// AC-027.3: один user на двух устройствах — 1 человек и 2 установки.
func TestUnit_PersonsVsInstalls(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "u-1", "2026-10-05T09:00:00+03:00"),
		session(t, 2, "u-1", "2026-10-05T18:00:00+03:00"),
	})

	require.Equal(t, uint64(1), count(t, env, period(t, identity.Persons, "2026-10-05", "2026-10-05"), qActive))
	require.Equal(t, uint64(2), count(t, env, period(t, identity.Installs, "2026-10-05", "2026-10-05"), qActive))
	require.Equal(t, map[string][]string{"2026-10-05": {installKey(1), installKey(2)}},
		personDays(t, env, period(t, identity.Installs, "2026-10-05", "2026-10-05")))
}

// AC-2.1.5: новая установка давнего пользователя не делает его новым человеком, но это новая установка.
func TestNew_PersonFirstSeenAcrossInstalls(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "", "2026-09-01T12:00:00+03:00"),
		session(t, 1, "u-1", "2026-09-02T12:00:00+03:00"),
		session(t, 2, "u-1", "2026-10-05T12:00:00+03:00"),
	})

	persons := period(t, identity.Persons, "2026-10-05", "2026-10-05")
	require.Equal(t, uint64(1), count(t, env, persons, qActive))
	require.Equal(t, uint64(0), count(t, env, persons, qNew))

	installs := period(t, identity.Installs, "2026-10-05", "2026-10-05")
	require.Equal(t, uint64(1), count(t, env, installs, qActive))
	require.Equal(t, uint64(1), count(t, env, installs, qNew))
}

// Второй аккаунт на чужом устройстве: first_seen берётся из user_first_seen его явных событий.
func TestNew_UserOnlyViaExplicitUserID(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "A", "2026-10-01T12:00:00+03:00"),
		session(t, 1, "B", "2026-10-04T12:00:00+03:00"),
	})

	day4 := period(t, identity.Persons, "2026-10-04", "2026-10-04")
	require.Equal(t, map[string][]string{"2026-10-04": {"u:B"}}, personDays(t, env, day4))
	require.Equal(t, uint64(1), count(t, env, day4, qNew), "B впервые появился 10-04")
	require.Equal(t, uint64(1), count(t, env, period(t, identity.Persons, "2026-10-01", "2026-10-01"), qNew))
	require.Equal(t, uint64(2), count(t, env, period(t, identity.Persons, "2026-10-01", "2026-10-04"), qNew))

	// В режиме установок B — не отдельная единица: установка одна и новая 10-01.
	require.Equal(t, uint64(0), count(t, env, period(t, identity.Installs, "2026-10-04", "2026-10-04"), qNew))
}

// environment отделяет и активность, и first_seen: отладка до релиза не делает человека старым в production.
func TestPersonsFirst_PerEnvironment(t *testing.T) {
	env := startWith(t, []chstore.EventRow{
		session(t, 1, "", "2026-10-05T12:00:00+03:00"),
		dev(session(t, 2, "", "2026-10-05T12:00:00+03:00")),
		dev(session(t, 3, "", "2026-09-01T12:00:00+03:00")),
		session(t, 3, "", "2026-10-05T13:00:00+03:00"),
	})

	prod := period(t, identity.Persons, "2026-10-05", "2026-10-05")
	require.Equal(t, map[string][]string{"2026-10-05": {installKey(1), installKey(3)}}, personDays(t, env, prod))
	require.Equal(t, uint64(2), count(t, env, prod, qNew))

	development := prod
	development.Env = EnvDevelopment
	require.Equal(t, map[string][]string{"2026-10-05": {installKey(2)}}, personDays(t, env, development))
	require.Equal(t, uint64(1), count(t, env, development, qNew))
}

// Канонические фрагменты на сценарии фикстур дают DAU и новых из таблицы README (модель testutil.ScenarioStats):
// общий девайс, досыл через 6 дней, сессии через полночь, дубли батчей, development и переход DST.
func TestIdentity_ScenarioMatchesModel(t *testing.T) {
	env := testutil.Start(t)
	rows := env.LoadNDJSON(t, testutil.ScenarioPath())

	for _, app := range testutil.ScenarioApps {
		t.Run(app.Code, func(t *testing.T) {
			want, err := testutil.ScenarioStats(rows, app)
			require.NoError(t, err)
			loc, err := time.LoadLocation(app.Timezone)
			require.NoError(t, err)
			p := Params{
				AppID:    app.ID,
				Location: loc,
				Env:      EnvProduction,
				Unit:     identity.Persons,
				Period:   Period{From: testutil.ScenarioFirstDay, To: testutil.ScenarioLastDay},
			}

			got := map[string]testutil.ScenarioDay{}
			res, err := env.CH.Conn().Query(p.Bind(t.Context()), qDaily)
			require.NoError(t, err)
			defer func() { _ = res.Close() }()
			for res.Next() {
				var day string
				var active, newcomers uint64
				require.NoError(t, res.Scan(&day, &active, &newcomers))
				got[day] = testutil.ScenarioDay{Day: day, Active: int(active), New: int(newcomers)}
			}
			require.NoError(t, res.Err())

			for _, w := range want {
				w.D1Returned = 0
				g, ok := got[w.Day]
				if !ok {
					g = testutil.ScenarioDay{Day: w.Day}
				}
				require.Equal(t, w, g, "день %s", w.Day)
				delete(got, w.Day)
			}
			require.Empty(t, got, "активность вне окна сценария")
		})
	}
}
