//go:build integration

package testutil_test

import (
	"context"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/testutil"
)

// scenarioDailySQL — DAU, новые и вернувшиеся на D1 по локальным дням приложения (production, unit=persons)
// по определениям 05-data-model.md §2.8 и §10.4: links → person_key, persons_first, cohort_f.
const scenarioDailySQL = `
WITH links AS (
    SELECT install_id, argMinMerge(first_user) AS linked_user
    FROM identity_links WHERE app_id = {app_id:UUID} GROUP BY install_id
),
persons_first AS (
    SELECT person_key, min(first_seen) AS first_seen
    FROM (
        SELECT if(l.linked_user = '', concat('i:', toString(f.install_id)), concat('u:', l.linked_user)) AS person_key,
               f.first_seen AS first_seen
        FROM (
            SELECT install_id, min(first_seen) AS first_seen FROM install_first_seen
            WHERE app_id = {app_id:UUID} AND environment = 'production' GROUP BY install_id
        ) AS f
        LEFT JOIN links AS l ON l.install_id = f.install_id
        UNION ALL
        SELECT concat('u:', user_id) AS person_key, min(first_seen) AS first_seen FROM user_first_seen
        WHERE app_id = {app_id:UUID} AND environment = 'production' GROUP BY user_id
    )
    GROUP BY person_key
),
act AS (
    SELECT DISTINCT
        if(e.user_id != '', concat('u:', e.user_id),
           if(l.linked_user != '', concat('u:', l.linked_user), concat('i:', toString(e.install_id)))) AS person_key,
        toDate(e.event_time, {tz:String}) AS day
    FROM events AS e LEFT JOIN links AS l ON l.install_id = e.install_id
    WHERE e.app_id = {app_id:UUID} AND e.environment = 'production'
)
SELECT toString(a.day) AS day,
       toUInt64(uniqExact(a.person_key)) AS active,
       toUInt64(countIf(toDate(pf.first_seen, {tz:String}) = a.day)) AS new,
       toUInt64(countIf(toDate(pf.first_seen, {tz:String}) = a.day
                        AND (a.person_key, a.day + 1) IN (SELECT person_key, day FROM act))) AS d1
FROM act AS a INNER JOIN persons_first AS pf ON pf.person_key = a.person_key
GROUP BY a.day ORDER BY a.day`

func TestFixtures_ScenarioInvariants(t *testing.T) {
	ctx := t.Context()
	env := testutil.Start(t)
	rows := env.LoadNDJSON(t, "testdata/fixtures/scenario.ndjson")
	conn := env.CH.Conn()

	// AC-026.1.
	var apps, installs, users, late uint64
	require.NoError(t, conn.QueryRow(ctx, `
		SELECT uniqExact(app_id), uniqExact(install_id), uniqExactIf(user_id, user_id != ''),
		       countIf(dateDiff('second', event_time, received_at) > 5 * 86400)
		FROM events`).Scan(&apps, &installs, &users, &late))
	require.Equal(t, uint64(3), apps)
	require.Equal(t, uint64(40), installs)
	require.Equal(t, uint64(12), users)
	require.Positive(t, late, "досыл: received_at − event_time > 5 дней")

	var shared uint64
	require.NoError(t, conn.QueryRow(ctx, `
		SELECT count() FROM (
			SELECT install_id FROM events WHERE user_id != ''
			GROUP BY install_id HAVING uniqExact(user_id) = 2
		)`).Scan(&shared))
	require.Equal(t, uint64(1), shared, "одна установка с двумя user_id")
	var moreThanTwo uint64
	require.NoError(t, conn.QueryRow(ctx, `
		SELECT count() FROM (
			SELECT install_id FROM events WHERE user_id != ''
			GROUP BY install_id HAVING uniqExact(user_id) > 2
		)`).Scan(&moreThanTwo))
	require.Zero(t, moreThanTwo)

	var devInstalls uint64
	require.NoError(t, conn.QueryRow(ctx,
		"SELECT uniqExactIf(install_id, environment = 'development') FROM events").Scan(&devInstalls))
	require.Positive(t, devInstalls)

	// Таблица README (модель в Go) совпадает с расчётом ClickHouse по каноническим определениям.
	for _, app := range testutil.ScenarioApps {
		t.Run(app.Code, func(t *testing.T) {
			want, err := testutil.ScenarioStats(rows, app)
			require.NoError(t, err)

			got := map[string]testutil.ScenarioDay{}
			res, err := conn.Query(clickhouseCtx(t, app), scenarioDailySQL)
			require.NoError(t, err)
			defer func() { _ = res.Close() }()
			for res.Next() {
				var day string
				var active, newcomers, d1 uint64
				require.NoError(t, res.Scan(&day, &active, &newcomers, &d1))
				got[day] = testutil.ScenarioDay{Day: day, Active: int(active), New: int(newcomers), D1Returned: int(d1)}
			}
			require.NoError(t, res.Err())

			for _, w := range want {
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

// clickhouseCtx передаёт app_id и tz как server-side параметры запроса.
func clickhouseCtx(t *testing.T, app testutil.ScenarioApp) context.Context {
	t.Helper()
	return clickhouse.Context(t.Context(), clickhouse.WithParameters(clickhouse.Parameters{
		"app_id": app.ID.String(),
		"tz":     app.Timezone,
	}))
}
