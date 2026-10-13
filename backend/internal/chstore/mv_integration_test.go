//go:build integration

package chstore_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

func TestIdentityLinksMV_FirstUserWins(t *testing.T) {
	ctx := t.Context()
	rw := openStore(t, rwUser, rwPassword)
	admin := adminConn(t)

	appID, installI, anonJ := uuid.New(), uuid.New(), uuid.New()
	t1, t2 := base, base.Add(time.Hour)

	userA := eventRow(appID, installI, t1)
	userA.UserID = "a"
	userB := eventRow(appID, installI, t2)
	userB.UserID = "b"
	anonymousI := eventRow(appID, installI, t2.Add(time.Hour))
	anonymousJ := eventRow(appID, anonJ, t1)

	// Позднего пользователя вставляем раньше: побеждает минимальный event_time, а не порядок вставки.
	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{userB, anonymousJ}))
	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{anonymousI}))
	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{userA}))

	var (
		firstUser            string
		firstLinked, lastSee time.Time
	)
	require.NoError(t, admin.QueryRow(ctx, `
		SELECT argMinMerge(first_user), min(first_linked_at), max(last_seen_at)
		FROM appmetrica.identity_links
		WHERE app_id = ? AND install_id = ?`, appID, installI).Scan(&firstUser, &firstLinked, &lastSee))
	require.Equal(t, "a", firstUser)
	require.True(t, t1.Equal(firstLinked), "first_linked_at = %v", firstLinked)
	// Анонимное событие в identity_links не попадает, поэтому last_seen_at — время события 'b'.
	require.True(t, t2.Equal(lastSee), "last_seen_at = %v", lastSee)

	var linksJ uint64
	require.NoError(t, admin.QueryRow(ctx,
		"SELECT count() FROM appmetrica.identity_links WHERE app_id = ? AND install_id = ?", appID, anonJ).Scan(&linksJ))
	require.Equal(t, uint64(0), linksJ, "события с user_id='' не создают строк")
}

func TestFirstSeenMVs(t *testing.T) {
	ctx := t.Context()
	rw := openStore(t, rwUser, rwPassword)
	admin := adminConn(t)

	appID, installID := uuid.New(), uuid.New()
	early := eventRow(appID, installID, base)
	late := eventRow(appID, installID, base.Add(48*time.Hour))
	late.UserID = "u1"
	dev := eventRow(appID, installID, base.Add(-time.Hour))
	dev.Environment = "development"

	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{late}))
	require.NoError(t, rw.InsertEvents(ctx, []chstore.EventRow{early, dev}))

	type seen struct {
		Environment string    `ch:"environment"`
		FirstSeen   time.Time `ch:"first_seen"`
		LastSeen    time.Time `ch:"last_seen"`
	}

	var installs []seen
	require.NoError(t, admin.Select(ctx, &installs, `
		SELECT environment, min(first_seen) AS first_seen, max(last_seen) AS last_seen
		FROM appmetrica.install_first_seen
		WHERE app_id = ? AND install_id = ?
		GROUP BY environment ORDER BY environment`, appID, installID))
	require.Len(t, installs, 2, "окружения считаются раздельно")
	require.Equal(t, "development", installs[0].Environment)
	require.True(t, dev.EventTime.Equal(installs[0].FirstSeen))
	require.Equal(t, "production", installs[1].Environment)
	require.True(t, early.EventTime.Equal(installs[1].FirstSeen))
	require.True(t, late.EventTime.Equal(installs[1].LastSeen))

	var users []struct {
		UserID    string    `ch:"user_id"`
		FirstSeen time.Time `ch:"first_seen"`
	}
	require.NoError(t, admin.Select(ctx, &users, `
		SELECT user_id, min(first_seen) AS first_seen
		FROM appmetrica.user_first_seen
		WHERE app_id = ?
		GROUP BY user_id`, appID))
	require.Len(t, users, 1, "пустой user_id в user_first_seen не пишется")
	require.Equal(t, "u1", users[0].UserID)
	require.True(t, late.EventTime.Equal(users[0].FirstSeen))
}
