package chstore_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/chstore"
)

func backupServer(t *testing.T, status int, body string) (*httptest.Server, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &paths
}

// GET /backup/list/remote отвечает JSONEachRow; created — «YYYY-MM-DD hh:mm:ss» в UTC контейнера.
func TestBackupClient_Latest(t *testing.T) {
	const list = `{"name":"full-2026-10-06T02-00-00","created":"2026-10-06 02:00:12","size":1024,"location":"remote","required":"","desc":"tar, regular"}
{"name":"increment-2026-10-13T02-00-00","created":"2026-10-13 02:00:41","size":10,"location":"remote","required":"full-2026-10-06T02-00-00","desc":"tar, regular"}
{"name":"increment-2026-10-12T02-00-00","created":"2026-10-12 02:00:33","size":10,"location":"remote","required":"full-2026-10-06T02-00-00","desc":"tar, regular"}
`
	srv, paths := backupServer(t, http.StatusOK, list)
	b, ok, err := chstore.NewBackupClient(srv.URL, nil).Latest(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, chstore.Backup{
		Name:      "increment-2026-10-13T02-00-00",
		CreatedAt: time.Date(2026, 10, 13, 2, 0, 41, 0, time.UTC),
	}, b)
	require.Equal(t, []string{"GET /backup/list/remote"}, *paths)
}

func TestBackupClient_LatestEdgeCases(t *testing.T) {
	t.Run("empty_list", func(t *testing.T) {
		srv, _ := backupServer(t, http.StatusOK, "")
		_, ok, err := chstore.NewBackupClient(srv.URL, nil).Latest(t.Context())
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("rfc3339_created", func(t *testing.T) {
		srv, _ := backupServer(t, http.StatusOK, `{"name":"b1","created":"2026-10-13T05:00:41+03:00"}`)
		b, ok, err := chstore.NewBackupClient(srv.URL, nil).Latest(t.Context())
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, time.Date(2026, 10, 13, 2, 0, 41, 0, time.UTC), b.CreatedAt.UTC())
	})

	// Битый бэкап (нет metadata.json) не считается свежим.
	t.Run("skips_broken", func(t *testing.T) {
		srv, _ := backupServer(t, http.StatusOK,
			`{"name":"ok","created":"2026-10-12 02:00:00","desc":"tar, regular"}`+"\n"+
				`{"name":"broken","created":"2026-10-13 02:00:00","desc":"broken (can't stat metadata.json)"}`)
		b, ok, err := chstore.NewBackupClient(srv.URL, nil).Latest(t.Context())
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, "ok", b.Name)
	})

	t.Run("server_error", func(t *testing.T) {
		srv, _ := backupServer(t, http.StatusInternalServerError, `{"status":"error","error":"s3 unavailable"}`)
		_, _, err := chstore.NewBackupClient(srv.URL, nil).Latest(t.Context())
		require.Error(t, err)
	})

	t.Run("bad_json", func(t *testing.T) {
		srv, _ := backupServer(t, http.StatusOK, `<html>`)
		_, _, err := chstore.NewBackupClient(srv.URL, nil).Latest(t.Context())
		require.Error(t, err)
	})

	t.Run("bad_created", func(t *testing.T) {
		srv, _ := backupServer(t, http.StatusOK, `{"name":"b1","created":"yesterday"}`)
		_, _, err := chstore.NewBackupClient(srv.URL, nil).Latest(t.Context())
		require.Error(t, err)
	})

	t.Run("unreachable", func(t *testing.T) {
		srv, _ := backupServer(t, http.StatusOK, "")
		srv.Close()
		_, _, err := chstore.NewBackupClient(srv.URL, nil).Latest(t.Context())
		require.Error(t, err)
	})

	t.Run("trailing_slash", func(t *testing.T) {
		srv, paths := backupServer(t, http.StatusOK, "")
		_, _, err := chstore.NewBackupClient(srv.URL+"/", nil).Latest(t.Context())
		require.NoError(t, err)
		require.Equal(t, []string{"GET /backup/list/remote"}, *paths)
	})
}
