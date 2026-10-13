//go:build integration

package reports

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/testutil"
)

// healthNow — «сейчас» тестов экрана «Интеграция»: 15:20 по Москве, окно 24 ч начинается в 16:00 MSK накануне
// (13:00 UTC).
var (
	healthNow         = time.Date(2026, 9, 10, 12, 20, 0, 0, time.UTC)
	healthWindowStart = time.Date(2026, 9, 9, 13, 0, 0, 0, time.UTC)
	healthOtherApp    = uuid.MustParse("1de0a000-0000-4000-8000-0000000000ff")
)

func healthParams(t testing.TB, tz string) Params {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	require.NoError(t, err)
	return Params{AppID: identityApp, Location: loc}
}

// acceptedEvent — session_start установки n версии version, принятый в at (event_time = received_at).
func acceptedEvent(t testing.TB, n int, version string, at time.Time) chstore.EventRow {
	t.Helper()
	r := session(t, n, "", at.Format(time.RFC3339Nano))
	r.AppVersion = version
	return r
}

// rejectedEvent — отклонённое событие приложения app с кодом code версии version, принятое в at.
func rejectedEvent(app uuid.UUID, code, version string, at time.Time) chstore.RejectedRow {
	return chstore.RejectedRow{
		ReceivedAt: at, AppID: app, KeyPrefix: "pk_live_1de0", Code: code, Field: "events[0]",
		Message: code + " в " + at.Format(time.TimeOnly), EventName: "session_start", AppVersion: version,
		Platform:      "ios",
		PayloadSample: fmt.Sprintf(`{"insert_id":%q,"name":"session_start","received":%q}`, uuid.New(), at.Format(time.RFC3339)),
	}
}

// startHealth поднимает базы теста, вставляет батчи событий (каждый — отдельной вставкой) и отказы.
// TTL-слияния rejected_events остановлены: отказы тестов старше 7 дней от настоящих часов.
func startHealth(t *testing.T, rejected []chstore.RejectedRow, batches ...[]chstore.EventRow) *testutil.Env {
	t.Helper()
	env := testutil.Start(t)
	require.NoError(t, env.CH.Conn().Exec(t.Context(), "SYSTEM STOP TTL MERGES rejected_events"))
	for _, b := range batches {
		require.NoError(t, env.CH.InsertEvents(t.Context(), b))
	}
	require.NoError(t, env.CH.InsertRejected(t.Context(), rejected))
	return env
}

// healthAt — отчёт на момент now с проверкой инвариантов: ровно hours точек, суммы точек равны итогам.
func healthAt(t *testing.T, env *testutil.Env, now time.Time, p Params, hours int) IntegrationHealth {
	t.Helper()
	got, err := NewService(env.CH.Conn(), clock.NewFixed(now)).IntegrationHealth(t.Context(), p, hours)
	require.NoError(t, err)
	require.Equal(t, hours, got.Hours)
	require.Len(t, got.Hourly, hours)
	require.NotNil(t, got.Reasons)
	require.NotNil(t, got.Versions)
	var accepted, rejected uint64
	for _, h := range got.Hourly {
		require.Equal(t, p.location(), h.Hour.Location())
		accepted += h.Accepted
		rejected += h.Rejected
	}
	require.Equal(t, got.AcceptedTotal, accepted, "accepted_total = сумма hourly")
	require.Equal(t, got.RejectedTotal, rejected, "rejected_total = сумма hourly")
	return got
}

// hourIndex — номер точки hourly для времени at в окне healthAt(…, healthNow, …, 24) приложения в Europe/Moscow.
func hourIndex(at time.Time) int {
	return int(at.Truncate(time.Hour).Sub(healthWindowStart) / time.Hour)
}

// AC-033.1 (AC-5.6.1): за 24 ч 100 принятых и 5 отклонённых (3 pii, 2 too_old) событий двух версий — суммы,
// 24 почасовые точки, 2 причины с последними примерами по убыванию count, 2 версии.
func TestIntegrationHealth_Aggregates(t *testing.T) {
	var events []chstore.EventRow
	wantAccepted := map[int]uint64{}
	for i := range 100 {
		version := "2.4.0"
		if i%5 < 2 {
			version = "2.3.0"
		}
		at := healthNow.Add(-time.Duration(i*13)*time.Minute - 5*time.Second)
		events = append(events, acceptedEvent(t, i+1, version, at))
		wantAccepted[hourIndex(at)]++
	}
	rejected := []chstore.RejectedRow{
		rejectedEvent(identityApp, "user_id_looks_like_pii", "2.4.0", healthNow.Add(-3*time.Hour)),
		rejectedEvent(identityApp, "user_id_looks_like_pii", "2.4.0", healthNow.Add(-time.Hour)),
		rejectedEvent(identityApp, "user_id_looks_like_pii", "2.4.0", healthNow.Add(-2*time.Hour)),
		rejectedEvent(identityApp, "event_too_old", "2.3.0", healthNow.Add(-20*time.Hour)),
		rejectedEvent(identityApp, "event_too_old", "2.3.0", healthNow.Add(-10*time.Hour)),
	}
	rejected[1].PayloadSample = `{"insert_id":"0199d6a4-5d6e-7f80-9a1b-2c3d4e5f6a7b","name":"session_start","user_id":"[redacted]"}`
	env := startHealth(t, rejected, events)

	got := healthAt(t, env, healthNow, healthParams(t, "Europe/Moscow"), 24)
	require.Equal(t, uint64(100), got.AcceptedTotal)
	require.Equal(t, uint64(5), got.RejectedTotal)
	require.NotNil(t, got.FirstEventAt)
	require.True(t, events[99].EventTime.Equal(*got.FirstEventAt), "first_event_at — самое раннее событие")

	for i, h := range got.Hourly {
		require.True(t, healthWindowStart.Add(time.Duration(i)*time.Hour).Equal(h.Hour), "час %d", i)
		require.Equal(t, wantAccepted[i], h.Accepted, "час %d", i)
	}
	require.Equal(t, "2026-09-09T16:00:00+03:00", got.Hourly[0].Hour.Format(time.RFC3339))
	require.Equal(t, "2026-09-10T15:00:00+03:00", got.Hourly[23].Hour.Format(time.RFC3339))
	for _, r := range rejected {
		require.Equal(t, uint64(1), got.Hourly[hourIndex(r.ReceivedAt)].Rejected, "отказ в %s", r.ReceivedAt)
	}

	require.Len(t, got.Reasons, 2)
	pii, old := got.Reasons[0], got.Reasons[1]
	require.Equal(t, "user_id_looks_like_pii", pii.Code)
	require.Equal(t, uint64(3), pii.Count)
	require.True(t, rejected[1].ReceivedAt.Equal(pii.LastSeen))
	require.Equal(t, rejected[1].PayloadSample, pii.ExamplePayload, "пример — последний payload_sample")
	require.Equal(t, rejected[1].Message, pii.ExampleMessage)
	require.Contains(t, pii.ExamplePayload, `"user_id":"[redacted]"`)
	require.Equal(t, "event_too_old", old.Code)
	require.Equal(t, uint64(2), old.Count)
	require.True(t, rejected[4].ReceivedAt.Equal(old.LastSeen))
	require.Equal(t, rejected[4].PayloadSample, old.ExamplePayload)
	require.Equal(t, rejected[4].Message, old.ExampleMessage)
	for _, r := range got.Reasons {
		require.NotContains(t, r.ExamplePayload, `"ip"`)
	}

	require.Len(t, got.Versions, 2)
	require.Equal(t, IntegrationVersion{AppVersion: "2.3.0", Accepted: 40, Rejected: 2, LastSeen: got.Versions[0].LastSeen}, got.Versions[0])
	require.True(t, events[0].ReceivedAt.Equal(got.Versions[0].LastSeen))
	require.Equal(t, IntegrationVersion{AppVersion: "2.4.0", Accepted: 60, Rejected: 3, LastSeen: got.Versions[1].LastSeen}, got.Versions[1])
	require.True(t, events[2].ReceivedAt.Equal(got.Versions[1].LastSeen))
}

// AC-033.2 (AC-5.6.3): отказы с нераспознанным ключом (nil app_id) и отказы и события другого приложения не
// попадают в отчёт приложения A.
func TestIntegrationHealth_IgnoresUnknownApp(t *testing.T) {
	at := healthNow.Add(-time.Hour)
	foreign := acceptedEvent(t, 1, "9.9.9", healthNow.Add(-48*time.Hour))
	foreign.AppID = healthOtherApp
	rejected := []chstore.RejectedRow{
		rejectedEvent(uuid.Nil, "invalid_app_key", "", at),
		rejectedEvent(uuid.Nil, "invalid_app_key", "", at.Add(time.Minute)),
		rejectedEvent(uuid.Nil, "event_too_old", "2.4.0", at.Add(2*time.Minute)),
		rejectedEvent(uuid.Nil, "event_too_old", "2.4.0", at.Add(3*time.Minute)),
		rejectedEvent(healthOtherApp, "invalid_field", "9.9.9", at),
		rejectedEvent(identityApp, "event_too_old", "2.4.0", at.Add(-time.Hour)),
	}
	env := startHealth(t, rejected, []chstore.EventRow{foreign, received(foreign, at)})

	got := healthAt(t, env, healthNow, healthParams(t, "Europe/Moscow"), 24)
	require.Nil(t, got.FirstEventAt, "события другого приложения не дают first_event_at")
	require.Zero(t, got.AcceptedTotal)
	require.Equal(t, uint64(1), got.RejectedTotal)
	require.Len(t, got.Reasons, 1)
	require.Equal(t, "event_too_old", got.Reasons[0].Code)
	require.Equal(t, uint64(1), got.Reasons[0].Count)
	require.True(t, rejected[5].ReceivedAt.Equal(got.Reasons[0].LastSeen))
	require.Equal(t, rejected[5].PayloadSample, got.Reasons[0].ExamplePayload)
	require.Equal(t, []IntegrationVersion{{AppVersion: "2.4.0", Rejected: 1, LastSeen: got.Versions[0].LastSeen}}, got.Versions)
}

// Тест-вектор waiting: приложение без событий — first_event_at = null, итоги 0, 24 нулевые точки.
func TestHealth_NoEventsYet(t *testing.T) {
	env := startHealth(t, nil)
	got := healthAt(t, env, healthNow, healthParams(t, "Europe/Moscow"), DefaultIntegrationHours)
	require.Nil(t, got.FirstEventAt)
	require.Zero(t, got.AcceptedTotal)
	require.Zero(t, got.RejectedTotal)
	require.Empty(t, got.Reasons)
	require.Empty(t, got.Versions)
}

// Тест-вектор first_event: first_event_at — время первого события приложения за всю историю в любом environment,
// даже вне окна hours.
func TestHealth_FirstEvent(t *testing.T) {
	first := dev(acceptedEvent(t, 1, "1.0.0", healthNow.Add(-72*time.Hour)))
	later := acceptedEvent(t, 2, "1.0.1", healthNow.Add(-30*time.Minute))
	env := startHealth(t, nil, []chstore.EventRow{later}, []chstore.EventRow{first})

	got := healthAt(t, env, healthNow, healthParams(t, "Europe/Moscow"), 24)
	require.NotNil(t, got.FirstEventAt)
	require.True(t, first.ReceivedAt.Equal(*got.FirstEventAt))
	require.Equal(t, time.UTC, got.FirstEventAt.Location())
	require.Equal(t, uint64(1), got.AcceptedTotal, "первое событие вне окна 24 ч")

	week := healthAt(t, env, healthNow, healthParams(t, "Europe/Moscow"), MaxIntegrationHours)
	require.Equal(t, uint64(2), week.AcceptedTotal, "development тоже считается")
	require.Equal(t, []string{"1.0.1", "1.0.0"}, versionNames(week.Versions))
}

// Тест-вектор reasons_grouped: 3 × event_too_old, 1 × invalid_field — 2 строки по count DESC; при равном count —
// по коду.
func TestHealth_ReasonsGrouped(t *testing.T) {
	at := healthNow.Add(-90 * time.Minute)
	env := startHealth(t, []chstore.RejectedRow{
		rejectedEvent(identityApp, "invalid_field", "3.4.0", at),
		rejectedEvent(identityApp, "event_too_old", "3.4.0", at.Add(time.Second)),
		rejectedEvent(identityApp, "event_too_old", "3.4.0", at.Add(2*time.Second)),
		rejectedEvent(identityApp, "event_too_old", "3.4.0", at.Add(3*time.Second)),
	})
	got := healthAt(t, env, healthNow, healthParams(t, "Europe/Moscow"), 24)
	require.Equal(t, []string{"event_too_old", "invalid_field"}, reasonCodes(got.Reasons))
	require.Equal(t, uint64(3), got.Reasons[0].Count)
	require.Equal(t, uint64(1), got.Reasons[1].Count)
	require.Equal(t, uint64(4), got.RejectedTotal)

	env = startHealth(t, []chstore.RejectedRow{
		rejectedEvent(identityApp, "invalid_field", "3.4.0", at),
		rejectedEvent(identityApp, "duration_required", "3.4.0", at.Add(time.Second)),
		rejectedEvent(identityApp, "app_rate_limited", "", at.Add(2*time.Second)),
		rejectedEvent(identityApp, "app_rate_limited", "", at.Add(3*time.Second)),
	})
	got = healthAt(t, env, healthNow, healthParams(t, "Europe/Moscow"), 24)
	require.Equal(t, []string{"app_rate_limited", "duration_required", "invalid_field"}, reasonCodes(got.Reasons))
}

// Тест-вектор versions: события 3.3.2 и 3.4.0 — строки с accepted/rejected по убыванию last_seen; версия только из
// отказов и пустая версия batch-ошибок — отдельные строки.
func TestHealth_ByVersion(t *testing.T) {
	at := healthNow.Add(-5 * time.Hour)
	env := startHealth(t, []chstore.RejectedRow{
		rejectedEvent(identityApp, "invalid_field", "3.3.2", at.Add(3*time.Hour)),
		rejectedEvent(identityApp, "invalid_field", "3.5.0-beta", at.Add(-time.Hour)),
		rejectedEvent(identityApp, "invalid_batch", "", at.Add(-2*time.Hour)),
	}, []chstore.EventRow{
		acceptedEvent(t, 1, "3.3.2", at),
		acceptedEvent(t, 2, "3.3.2", at.Add(time.Hour)),
		acceptedEvent(t, 3, "3.4.0", at.Add(time.Minute)),
		acceptedEvent(t, 4, "3.4.0", at.Add(4*time.Hour)),
		acceptedEvent(t, 5, "3.4.0", at.Add(2*time.Minute)),
	})
	got := healthAt(t, env, healthNow, healthParams(t, "Europe/Moscow"), 24)
	require.Equal(t, uint64(5), got.AcceptedTotal)
	require.Equal(t, uint64(3), got.RejectedTotal)
	want := []IntegrationVersion{
		{AppVersion: "3.4.0", Accepted: 3, LastSeen: at.Add(4 * time.Hour)},
		{AppVersion: "3.3.2", Accepted: 2, Rejected: 1, LastSeen: at.Add(3 * time.Hour)},
		{AppVersion: "3.5.0-beta", Rejected: 1, LastSeen: at.Add(-time.Hour)},
		{AppVersion: "", Rejected: 1, LastSeen: at.Add(-2 * time.Hour)},
	}
	require.Len(t, got.Versions, len(want))
	for i, w := range want {
		g := got.Versions[i]
		require.Equal(t, w.AppVersion, g.AppVersion, "версия %d", i)
		require.Equal(t, w.Accepted, g.Accepted, "версия %q", w.AppVersion)
		require.Equal(t, w.Rejected, g.Rejected, "версия %q", w.AppVersion)
		require.True(t, w.LastSeen.Equal(g.LastSeen), "версия %q: last_seen %s", w.AppVersion, g.LastSeen)
	}
}

// Границы окна: [начало первого часа, now] по received_at; поздняя доставка считается по received_at; ретрай
// с тем же insert_id — одно событие в часе последнего приёма, как после слияния ReplacingMergeTree(received_at).
func TestHealth_WindowAndRetries(t *testing.T) {
	inWindow := acceptedEvent(t, 1, "1.0.0", healthWindowStart)
	beforeWindow := acceptedEvent(t, 2, "1.0.0", healthWindowStart.Add(-time.Millisecond))
	afterNow := acceptedEvent(t, 3, "1.0.0", healthNow.Add(time.Second))
	atNow := acceptedEvent(t, 4, "1.0.0", healthNow)
	late := acceptedEvent(t, 5, "1.0.0", healthNow.Add(-5*24*time.Hour))
	late = received(late, healthNow.Add(-time.Hour))
	retry := acceptedEvent(t, 6, "1.0.0", healthNow.Add(-5*time.Hour))
	retried := received(retry, healthNow.Add(-2*time.Hour))
	devEvent := dev(acceptedEvent(t, 7, "1.0.0", healthNow.Add(-3*time.Hour)))

	env := startHealth(t, []chstore.RejectedRow{
		rejectedEvent(identityApp, "invalid_field", "1.0.0", healthWindowStart),
		rejectedEvent(identityApp, "invalid_field", "1.0.0", healthWindowStart.Add(-time.Millisecond)),
		rejectedEvent(identityApp, "invalid_field", "1.0.0", healthNow.Add(time.Second)),
	},
		[]chstore.EventRow{inWindow, beforeWindow, afterNow, atNow, late, retry, devEvent},
		[]chstore.EventRow{retried},
	)

	got := healthAt(t, env, healthNow, healthParams(t, "Europe/Moscow"), 24)
	require.Equal(t, uint64(5), got.AcceptedTotal, "inWindow, atNow, late, retry, devEvent")
	require.Equal(t, uint64(1), got.RejectedTotal)
	require.Equal(t, uint64(1), got.Hourly[0].Accepted)
	require.Equal(t, uint64(1), got.Hourly[0].Rejected)
	require.Equal(t, uint64(1), got.Hourly[hourIndex(healthNow)].Accepted, "atNow")
	require.Equal(t, uint64(1), got.Hourly[hourIndex(late.ReceivedAt)].Accepted, "late — по received_at")
	require.Equal(t, uint64(1), got.Hourly[hourIndex(retried.ReceivedAt)].Accepted, "ретрай — в часе последнего приёма")
	require.Zero(t, got.Hourly[hourIndex(retry.ReceivedAt)].Accepted, "первый приём ретрая не считается")
	require.Equal(t, uint64(1), got.Hourly[hourIndex(devEvent.ReceivedAt)].Accepted)
	require.True(t, late.EventTime.Equal(*got.FirstEventAt), "first_event_at — по event_time первого события")
	require.Equal(t, []IntegrationVersion{{AppVersion: "1.0.0", Accepted: 5, Rejected: 1, LastSeen: got.Versions[0].LastSeen}}, got.Versions)
	require.True(t, healthNow.Equal(got.Versions[0].LastSeen))
}

// Часы окна — в TZ приложения: у Asia/Kolkata (+05:30) час начинается в :30 UTC.
func TestHealth_HalfHourTimezone(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 20, 30, 0, time.UTC) // 17:50:30 IST
	env := startHealth(t, nil, []chstore.EventRow{
		acceptedEvent(t, 1, "1.0.0", time.Date(2026, 9, 10, 10, 29, 59, 0, time.UTC)), // 15:59:59 IST — до окна
		acceptedEvent(t, 2, "1.0.0", time.Date(2026, 9, 10, 10, 30, 0, 0, time.UTC)),  // 16:00 IST
		acceptedEvent(t, 3, "1.0.0", time.Date(2026, 9, 10, 11, 29, 59, 0, time.UTC)), // 16:59:59 IST
		acceptedEvent(t, 4, "1.0.0", time.Date(2026, 9, 10, 11, 30, 0, 0, time.UTC)),  // 17:00 IST
	})
	got := healthAt(t, env, now, healthParams(t, "Asia/Kolkata"), 2)
	require.Equal(t, "2026-09-10T16:00:00+05:30", got.Hourly[0].Hour.Format(time.RFC3339))
	require.Equal(t, uint64(2), got.Hourly[0].Accepted)
	require.Equal(t, uint64(1), got.Hourly[1].Accepted)
	require.Equal(t, uint64(3), got.AcceptedTotal)
}

func reasonCodes(rs []IntegrationReason) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Code)
	}
	return out
}

func versionNames(vs []IntegrationVersion) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.AppVersion)
	}
	return out
}
