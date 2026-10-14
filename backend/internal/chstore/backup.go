package chstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// backupCreatedFormat — поле created в ответе clickhouse-backup (common.TimeFormat); часовой пояс контейнера — UTC.
const backupCreatedFormat = "2006-01-02 15:04:05"

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

// NewBackupClient — httpClient nil — http.DefaultClient; таймаут задаёт контекст запроса.
func NewBackupClient(baseURL string, httpClient *http.Client) *BackupClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &BackupClient{baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

type backupListItem struct {
	Name    string `json:"name"`
	Created string `json:"created"`
	Desc    string `json:"desc"`
}

// Latest — самый свежий исправный удалённый бэкап (GET /backup/list/remote, JSONEachRow); ok=false — бэкапов нет.
func (c *BackupClient) Latest(ctx context.Context) (b Backup, ok bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/backup/list/remote", nil)
	if err != nil {
		return Backup{}, false, fmt.Errorf("chstore: backup list: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Backup{}, false, fmt.Errorf("chstore: backup list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Backup{}, false, fmt.Errorf("chstore: backup list: HTTP %d", resp.StatusCode)
	}

	dec := json.NewDecoder(resp.Body)
	for {
		var item backupListItem
		if err := dec.Decode(&item); errors.Is(err, io.EOF) {
			return b, ok, nil
		} else if err != nil {
			return Backup{}, false, fmt.Errorf("chstore: backup list: decode: %w", err)
		}
		if strings.HasPrefix(item.Desc, "broken") {
			continue
		}
		created, err := parseBackupCreated(item.Created)
		if err != nil {
			return Backup{}, false, fmt.Errorf("chstore: backup list: %s: %w", item.Name, err)
		}
		if !ok || created.After(b.CreatedAt) {
			b, ok = Backup{Name: item.Name, CreatedAt: created}, true
		}
	}
}

func parseBackupCreated(s string) (time.Time, error) {
	if t, err := time.ParseInLocation(backupCreatedFormat, s, time.UTC); err == nil {
		return t, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid created %q", s)
	}
	return t, nil
}
