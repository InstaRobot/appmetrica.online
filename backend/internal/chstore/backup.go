package chstore

import (
	"context"
	"net/http"
	"time"
)

// Backup — удалённый бэкап clickhouse-backup.
type Backup struct {
	Name      string
	CreatedAt time.Time
}

// BackupClient читает список бэкапов из REST API clickhouse-backup (BACKUP_API_URL).
type BackupClient struct {
	baseURL string
	http    *http.Client
}

// NewBackupClient — baseURL без завершающего «/»; httpClient nil — http.DefaultClient.
func NewBackupClient(baseURL string, httpClient *http.Client) *BackupClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &BackupClient{baseURL: baseURL, http: httpClient}
}

// Latest — самый свежий удалённый бэкап; ok=false — бэкапов нет.
func (c *BackupClient) Latest(_ context.Context) (b Backup, ok bool, err error) {
	return Backup{}, false, errNotImplemented
}
