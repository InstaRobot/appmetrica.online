//go:build integration

package merge_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/clock"
	"appmetrica.online/backend/internal/merge"
)

// AC-002.3: шаг copy переносит каждую колонку events, включая properties. Список колонок берётся из
// system.columns, а не из кода: колонка, которую забыли в INSERT/SELECT copy.go, роняет тест, а не теряет
// данные источника молча. Строка источника обязана заполнять все колонки не значением по умолчанию —
// новая колонка без значения в тесте тоже падает.
func TestCopyEvents_AllColumns(t *testing.T) {
	e := newEnv(t)
	ctx := t.Context()
	target := e.create(t, "Notes iOS", []string{"ios"}, "UTC")
	source := e.create(t, "Notes Android", []string{"android"}, "UTC")

	at := time.Date(2026, 9, 3, 10, 0, 0, 123_000_000, time.UTC)
	row := events(source, "android", "u1", 1, at, time.Hour)[0]
	row.Name, row.DurationMs = "purchase", 4321
	row.ClientTime, row.SentAt, row.ReceivedAt = at.Add(-time.Hour), at.Add(time.Second), at.Add(2*time.Second)
	row.OSName, row.OSVersion, row.AppVersion, row.AppBuild = "Android", "16", "2.1.0", "210"
	row.DeviceModel, row.DeviceType, row.Locale = "Pixel 10", "phone", "de-DE"
	row.Country, row.Region, row.City = "DE", "Berlin", "Berlin"
	row.WriteKeyID = uuid.New()
	row.Properties = map[string]string{"plan": "12", "paid": "true"}
	require.NoError(t, e.CH.InsertEvents(ctx, []chstore.EventRow{row}))
	e.closeDays(t, target, at, at)
	e.createMerge(t, target, source)

	type column struct{ Name, Type string }
	var cols []column
	rows, err := e.CH.Conn().Query(ctx, `SELECT name, type FROM system.columns
		WHERE database = currentDatabase() AND table = 'events' ORDER BY position`)
	require.NoError(t, err)
	for rows.Next() {
		var c column
		require.NoError(t, rows.Scan(&c.Name, &c.Type))
		cols = append(cols, c)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	var names []string
	for _, c := range cols {
		names = append(names, c.Name)
	}
	require.Contains(t, names, "properties")

	value := func(app appJSON, c column) (v string, isDefault bool) {
		q := "SELECT toString(" + c.Name + "), toString(" + c.Name + ") = toString(defaultValueOfTypeName('" +
			strings.ReplaceAll(c.Type, "'", `\'`) + "')) FROM events WHERE app_id = {app:UUID}"
		qctx := clickhouse.Context(ctx, clickhouse.WithParameters(clickhouse.Parameters{"app": app.ID}))
		require.NoError(t, e.CH.Conn().QueryRow(qctx, q).Scan(&v, &isDefault), c.Name)
		return v, isDefault
	}
	before := map[string]string{}
	for _, c := range cols {
		v, isDefault := value(source, c)
		require.False(t, isDefault, "тестовая строка источника не заполняет колонку %s", c.Name)
		before[c.Name] = v
	}

	var failed []failedMerge
	runToDone(t, e.executor(clock.NewFixed(t0), &failed, merge.WithKeysWait(0)), uuid.New())
	require.Empty(t, failed)

	require.Equal(t, uint64(1), e.appRows(t, "events", target))
	for _, c := range cols {
		if c.Name == "app_id" {
			continue
		}
		v, _ := value(target, c)
		require.Equal(t, before[c.Name], v, "колонка %s не перенесена", c.Name)
	}
	require.Equal(t, "{'paid':'true','plan':'12'}", before["properties"])
}
