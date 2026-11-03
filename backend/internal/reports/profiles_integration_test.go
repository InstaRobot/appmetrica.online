//go:build integration

package reports

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

// profileEvent — своё событие name установки n пользователя user в at (RFC 3339).
func profileEvent(t testing.TB, n int, user, at, name string, props map[string]string) chstore.EventRow {
	t.Helper()
	r := session(t, n, user, at)
	r.Name, r.Properties = name, props
	return r
}

func profileEvents(t *testing.T, svc *Service, q ProfileEventsQuery) ProfileEventPage {
	t.Helper()
	got, err := svc.ProfileEvents(t.Context(), identityApp, q)
	require.NoError(t, err)
	require.NotNil(t, got.Events)
	return got
}

// AC-007.1: человек с двумя установками и 130 своими событиями (по три на одну секунду — порядок внутри секунды
// задаёт insert_id), системные события, события другого человека и повтор батча (тот же insert_id) страниц не
// меняют: 50 + 50 + 30 без повторов и пропусков по убыванию (event_time, insert_id).
func TestProfileEvents_Pagination(t *testing.T) {
	start := ts(t, "2026-10-05T10:00:00Z")
	var mine, rows []chstore.EventRow
	for i := range 130 {
		at := start.Add(time.Duration(i/3) * time.Second).Format(time.RFC3339)
		mine = append(mine, profileEvent(t, 1+i%2, "U", at, "note_created", map[string]string{"source": fmt.Sprint(i)}))
	}
	rows = append(rows, mine...)
	rows = append(rows, mine[7], mine[100]) // повтор батча: тот же insert_id
	rows = append(rows, visit(t, 1, "U", "2026-10-05T10:00:05Z", time.Minute)...)
	rows = append(rows, profileEvent(t, 3, "", "2026-10-05T10:00:06Z", "note_created", nil)) // другой человек
	rows = append(rows, profileEvent(t, 4, "V", "2026-10-05T10:00:07Z", "note_created", nil))
	svc := newService(startWith(t, rows[:100], rows[100:]))

	var got []ProfileEvent
	q := ProfileEventsQuery{PersonKey: "u:U", Limit: 50}
	for _, wantLen := range []int{50, 50, 30} {
		page := profileEvents(t, svc, q)
		require.Len(t, page.Events, wantLen)
		got = append(got, page.Events...)
		if wantLen == 30 {
			require.Empty(t, page.NextBefore, "последняя страница")
			break
		}
		require.NotEmpty(t, page.NextBefore)
		q.Before = page.NextBefore
	}

	require.Len(t, got, 130, "повтор батча — одна запись")
	seen := map[string]bool{}
	for i, e := range got {
		require.Equal(t, "note_created", e.Name)
		require.False(t, seen[e.Properties["source"]], "событие %s повторено", e.Properties["source"])
		seen[e.Properties["source"]] = true
		if i > 0 {
			require.False(t, e.EventTime.After(got[i-1].EventTime), "время не растёт")
		}
	}
	require.Len(t, seen, 130)
	installs := map[uuid.UUID]bool{}
	for _, e := range got {
		installs[e.InstallID] = true
	}
	require.Equal(t, map[uuid.UUID]bool{install(1): true, install(2): true}, installs)
	first := got[0]
	require.Equal(t, "ios", first.Platform)
	require.NotEqual(t, uuid.Nil, first.SessionID)
}

// События всех установок человека; анонимная установка — свой человек i:<id>; environment фильтрует события,
// по умолчанию production; limit по умолчанию 50.
func TestProfileEvents_InstallsAndEnvironment(t *testing.T) {
	rows := concat(
		visit(t, 1, "", "2026-10-01T09:00:00Z", time.Minute),
		[]chstore.EventRow{
			profileEvent(t, 1, "U", "2026-10-01T10:00:00Z", "a", nil),
			profileEvent(t, 2, "U", "2026-10-01T11:00:00Z", "b", map[string]string{"k": "v"}),
			dev(profileEvent(t, 2, "U", "2026-10-01T12:00:00Z", "c", nil)),
			profileEvent(t, 3, "", "2026-10-01T13:00:00Z", "d", nil),
		},
	)
	svc := newService(startWith(t, rows))

	page := profileEvents(t, svc, ProfileEventsQuery{PersonKey: "u:U"})
	require.Len(t, page.Events, 2)
	require.Equal(t, "b", page.Events[0].Name)
	require.Equal(t, install(2), page.Events[0].InstallID)
	require.Equal(t, map[string]string{"k": "v"}, page.Events[0].Properties)
	require.Equal(t, "a", page.Events[1].Name)
	require.NotNil(t, page.Events[1].Properties, "без свойств — пустой объект")
	require.Empty(t, page.NextBefore)

	page = profileEvents(t, svc, ProfileEventsQuery{PersonKey: "u:U", Environment: EnvDevelopment})
	require.Len(t, page.Events, 1)
	require.Equal(t, "c", page.Events[0].Name)

	page = profileEvents(t, svc, ProfileEventsQuery{PersonKey: installKey(3)})
	require.Len(t, page.Events, 1)
	require.Equal(t, "d", page.Events[0].Name)
}

// Нет такого человека — ErrProfileNotFound; у человека только системные события — пустая страница; плохой limit
// и курсор — ErrUnsupported и ErrInvalidCursor.
func TestProfileEvents_Errors(t *testing.T) {
	svc := newService(startWith(t, visit(t, 1, "U", "2026-10-01T09:00:00Z", time.Minute)))
	ctx := t.Context()

	_, err := svc.ProfileEvents(ctx, identityApp, ProfileEventsQuery{PersonKey: "u:nobody"})
	require.ErrorIs(t, err, ErrProfileNotFound)

	page := profileEvents(t, svc, ProfileEventsQuery{PersonKey: "u:U"})
	require.Empty(t, page.Events)
	require.Empty(t, page.NextBefore)

	for _, limit := range []int{-1, 201} {
		_, err = svc.ProfileEvents(ctx, identityApp, ProfileEventsQuery{PersonKey: "u:U", Limit: limit})
		require.ErrorIs(t, err, ErrUnsupported, "limit=%d", limit)
	}
	_, err = svc.ProfileEvents(ctx, identityApp, ProfileEventsQuery{PersonKey: "u:U", Before: "garbage"})
	require.ErrorIs(t, err, ErrInvalidCursor)
}
