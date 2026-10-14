package docscheck_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/docscheck"
	"appmetrica.online/backend/internal/testutil"
)

const (
	// prodIngestBase — адрес ingest в сниппетах документации; тест подставляет локальный сервер.
	prodIngestBase = "https://i.appmetrica.online"
	// devWriteKey — DEV_WRITE_KEY из .env.example: write-ключ dev-приложения из seed.
	devWriteKey = "pk_devseedkey0000000000000000000000"

	snippetTimeout = 30 * time.Second
)

// docsDir — web/content/docs: MDX-гайды и рецепты S-13.
func docsDir() string {
	return filepath.Join(testutil.BackendDir(), "..", "web", "content", "docs")
}

func docSnippets(t *testing.T) []docscheck.Snippet {
	t.Helper()
	snips, err := docscheck.ExtractDir(docsDir())
	require.NoError(t, err)
	return snips
}

// filesOf — MDX-файлы, в которых есть сниппеты.
func filesOf(snips []docscheck.Snippet) []string {
	var files []string
	for _, s := range snips {
		if len(files) == 0 || files[len(files)-1] != s.File {
			files = append(files, s.File)
		}
	}
	return files
}

// AC-048.1: каждый блок ```go в MDX — самостоятельный файл Go без сторонних модулей, который компилируется
// и проходит go vet.
func TestDocsGoSnippets_Vet(t *testing.T) {
	snips := docscheck.ByLang(docSnippets(t), "go")
	require.Subset(t, filesOf(snips), []string{"recipes/bots-and-backends.mdx", "recipes/go.mdx"})
	for _, s := range snips {
		require.Contains(t, s.Code, "package ", "%s: Go-сниппет должен быть полным файлом", s.Pos())
	}
	require.NoError(t, vetGo(t, snips))
}

// AC-048.2: каждый curl-сниппет, который отправляет батч в ingest, с ключом DEV_WRITE_KEY получает 200
// и accepted > 0.
func TestDocsCurlSnippets_Accepted(t *testing.T) {
	snips := ingestCurlSnippets(docSnippets(t))
	require.Subset(t, filesOf(snips), []string{"index.mdx", "recipes/curl.mdx"})
	requireTools(t, "bash", "curl", "uuidgen", "gzip", "date", "jq")

	target := startIngest(t)
	total := 0
	for _, s := range snips {
		require.Contains(t, s.Code, "$AM_PK", "%s: ключ берётся из переменной AM_PK", s.Pos())
		out, err := runShellSnippet(t, s, target.url, devWriteKey)
		require.NoError(t, err, "%s\n%s", s.Pos(), out)
		accepted, err := checkResponses(target.take())
		require.NoError(t, err, "%s\n%s", s.Pos(), out)
		total += accepted
	}
	target.verify(t, total)
}

// Скрипт повторов из recipes/curl.mdx: 429 и 503 — пауза из Retry-After, 5xx — экспоненциальная задержка,
// повтор несёт те же insert_id и новый sent_at; 4xx, кроме 429, не повторяется и завершает скрипт ошибкой.
func TestDocsCurlRetrySnippet(t *testing.T) {
	requireTools(t, "bash", "curl", "uuidgen", "date")
	s := curlSnippetWith(t, "recipes/curl.mdx", "Retry-After")

	tests := []struct {
		name     string
		statuses []int
		wantErr  bool
	}{
		{name: "503 waits for Retry-After", statuses: []int{http.StatusServiceUnavailable, http.StatusOK}},
		{name: "429 waits for Retry-After", statuses: []int{http.StatusTooManyRequests, http.StatusOK}},
		{name: "500 backs off", statuses: []int{http.StatusInternalServerError, http.StatusOK}},
		{name: "400 is not retried", statuses: []int{http.StatusBadRequest}, wantErr: true},
		{name: "401 is not retried", statuses: []int{http.StatusUnauthorized}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newScriptedIngest(t, tt.statuses)
			out, err := runShellSnippet(t, s, srv.url, devWriteKey)
			reqs := srv.requests()
			if tt.wantErr {
				require.Error(t, err, out)
				require.Len(t, reqs, 1, "батч с ответом %d не повторяется\n%s", tt.statuses[0], out)
				return
			}
			require.NoError(t, err, out)
			require.Len(t, reqs, len(tt.statuses), out)
			first, retry := reqs[0], reqs[1]
			require.NotEmpty(t, first.InsertIDs())
			require.Equal(t, first.InsertIDs(), retry.InsertIDs(), "повтор с теми же insert_id")
			require.True(t, retry.SentAt.After(first.SentAt), "sent_at выставляется при каждой попытке: %s → %s",
				first.SentAt, retry.SentAt)
		})
	}
}

func TestVetGo_ReportsMDXPosition(t *testing.T) {
	requireTools(t, "go")
	ok := docscheck.Snippet{File: "recipes/ok.mdx", Line: 3, Lang: "go", Code: "package main\n\nfunc main() {}\n"}
	printf := docscheck.Snippet{File: "recipes/x.mdx", Line: 10, Lang: "go", Code: "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Printf(\"%d\\n\", \"x\")\n}\n"}
	undefined := docscheck.Snippet{File: "recipes/y.mdx", Line: 20, Lang: "go", Code: "package main\n\nfunc main() {\n\tmissing()\n}\n"}
	foreign := docscheck.Snippet{File: "recipes/z.mdx", Line: 30, Lang: "go", Code: "package main\n\nimport \"github.com/google/uuid\"\n\nvar _ = uuid.New\n"}

	require.NoError(t, vetGo(t, []docscheck.Snippet{ok}))

	err := vetGo(t, []docscheck.Snippet{ok, printf})
	require.ErrorContains(t, err, "recipes/x.mdx:16:")
	require.ErrorContains(t, err, "Printf format %d has arg \"x\" of wrong type string")
	require.NotContains(t, err.Error(), "recipes/ok.mdx")

	err = vetGo(t, []docscheck.Snippet{undefined})
	require.ErrorContains(t, err, "recipes/y.mdx:24:")
	require.ErrorContains(t, err, "undefined: missing")

	// Документация не тянет сторонние модули: сниппет должен собираться одной стандартной библиотекой.
	err = vetGo(t, []docscheck.Snippet{foreign})
	require.ErrorContains(t, err, "recipes/z.mdx")
	require.ErrorContains(t, err, "github.com/google/uuid")
}

func TestIngestCurlSnippets_Selection(t *testing.T) {
	snips := []docscheck.Snippet{
		{File: "a.mdx", Line: 1, Lang: "bash", Code: "curl -sS https://i.appmetrica.online/v1/events -H \"X-App-Key: $AM_PK\"\n"},
		{File: "a.mdx", Line: 5, Lang: "sh", Code: "curl -sS \"https://i.appmetrica.online/v1/events\"\n"},
		{File: "b.mdx", Line: 1, Lang: "bash", Code: "curl -sS https://api.appmetrica.online/v1/apps -H \"Authorization: Bearer $AM_SK\"\n"},
		{File: "b.mdx", Line: 9, Lang: "bash", Code: "export AM_PK=pk_…\n"},
		{File: "c.mdx", Line: 1, Lang: "go", Code: "const endpoint = \"https://i.appmetrica.online/v1/events\"\n"},
		{File: "c.mdx", Line: 7, Lang: "http", Code: "POST https://i.appmetrica.online/v1/events\n"},
	}
	require.Equal(t, []docscheck.Snippet{snips[0], snips[1]}, ingestCurlSnippets(snips))
}

func TestCurlSnippets_CheckResponses(t *testing.T) {
	requireTools(t, "bash", "curl")
	target := startIngest(t)
	run := func(code string) ([]response, string) {
		t.Helper()
		s := docscheck.Snippet{File: "t.mdx", Line: 1, Lang: "bash", Code: code}
		out, err := runShellSnippet(t, s, target.url, devWriteKey)
		require.NoError(t, err, out)
		return target.take(), out
	}
	batch := func(ts string) string {
		return `curl -sS https://i.appmetrica.online/v1/events -H "Content-Type: application/json" -H "X-App-Key: $AM_PK" ` +
			`--data-binary '{"sent_at":"` + ts + `","context":{"environment":"development"},"events":[{"insert_id":"0199d6a1-7c2e-7b1a-9f00-3c1d2e4f5a61","name":"session_start","timestamp":"` + ts + `","install_id":"6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21","session_id":"0199d6a1-7b90-7e3c-8d11-5a6b7c8d9e0f"}]}'` + "\n"
	}
	now := time.Now().UTC().Format(time.RFC3339)

	reqs, _ := run(batch(now))
	accepted, err := checkResponses(reqs)
	require.NoError(t, err)
	require.Equal(t, 1, accepted)

	reqs, _ = run("true\n")
	_, err = checkResponses(reqs)
	require.EqualError(t, err, "сниппет не отправил ни одного запроса в ingest")

	reqs, _ = run(`curl -sS https://i.appmetrica.online/v1/events -H "Content-Type: application/json" -H "X-App-Key: $AM_PK" --data-binary '{"sent_at":'` + "\n")
	_, err = checkResponses(reqs)
	require.ErrorContains(t, err, "запрос 1: HTTP 400")
	require.ErrorContains(t, err, "invalid_json")

	reqs, _ = run(`curl -sS https://i.appmetrica.online/v1/events --data-binary '{}'` + "\n")
	_, err = checkResponses(reqs)
	require.ErrorContains(t, err, "запрос 1: HTTP 415")

	// Событие старше 7 дней: 200, но accepted = 0.
	reqs, _ = run(batch(now) + strings.Replace(batch("2020-01-01T00:00:00Z"), `"sent_at":"2020-01-01T00:00:00Z"`, `"sent_at":"`+now+`"`, 1))
	_, err = checkResponses(reqs)
	require.ErrorContains(t, err, "запрос 2: accepted = 0")
	require.ErrorContains(t, err, "event_too_old")

	s := docscheck.Snippet{File: "t.mdx", Line: 1, Lang: "bash", Code: "echo broken >&2\nexit 3\n"}
	out, err := runShellSnippet(t, s, target.url, devWriteKey)
	require.Error(t, err)
	require.Contains(t, out, "broken")
}

// ingestCurlSnippets — shell-блоки, которые обращаются к ingest по боевому адресу.
func ingestCurlSnippets(snips []docscheck.Snippet) []docscheck.Snippet {
	var out []docscheck.Snippet
	for _, s := range docscheck.ByLang(snips, "bash", "sh", "shell") {
		if strings.Contains(s.Code, "curl") && strings.Contains(s.Code, prodIngestBase+"/v1/events") {
			out = append(out, s)
		}
	}
	return out
}

// curlSnippetWith — единственный ingest-сниппет файла, в коде которого есть marker.
func curlSnippetWith(t *testing.T, file, marker string) docscheck.Snippet {
	t.Helper()
	var found []docscheck.Snippet
	for _, s := range ingestCurlSnippets(docSnippets(t)) {
		if s.File == file && strings.Contains(s.Code, marker) {
			found = append(found, s)
		}
	}
	require.Len(t, found, 1, "%s: ровно один сниппет с %q", file, marker)
	return found[0]
}

// sentBatch — батч, который получил scriptedIngest.
type sentBatch struct {
	SentAt time.Time `json:"sent_at"`
	Events []struct {
		InsertID string `json:"insert_id"`
	} `json:"events"`
}

func (b sentBatch) InsertIDs() []string {
	ids := make([]string, 0, len(b.Events))
	for _, e := range b.Events {
		ids = append(ids, e.InsertID)
	}
	return ids
}

// scriptedIngest отвечает на POST статусами по порядку (последний — на все следующие запросы): 429 и 503 —
// с Retry-After: 1, 200 — с IngestResult, остальные — с конвертом error.
type scriptedIngest struct {
	url string

	mu      sync.Mutex
	batches []sentBatch
}

func newScriptedIngest(t *testing.T, statuses []int) *scriptedIngest {
	t.Helper()
	si := &scriptedIngest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b sentBatch
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, err.Error(), http.StatusTeapot)
			return
		}
		si.mu.Lock()
		si.batches = append(si.batches, b)
		status := statuses[min(len(si.batches), len(statuses))-1]
		si.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
			w.Header().Set("Retry-After", "1")
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = fmt.Fprintf(w, `{"accepted":%d,"rejected":[],"warnings":[],"server_time":%q}`,
				len(b.Events), time.Now().UTC().Format(time.RFC3339Nano))
			return
		}
		_, _ = fmt.Fprintf(w, `{"error":{"code":"http_%d","message":"scripted","request_id":"test"}}`, status)
	}))
	t.Cleanup(srv.Close)
	si.url = srv.URL
	return si
}

func (si *scriptedIngest) requests() []sentBatch {
	si.mu.Lock()
	defer si.mu.Unlock()
	return slices.Clone(si.batches)
}

func requireTools(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		_, err := exec.LookPath(tool)
		require.NoError(t, err, "для проверки сниппетов нужен %s (uuidgen — пакет uuid-runtime)", tool)
	}
}

// vetGo раскладывает сниппеты по пакетам временного модуля без зависимостей и запускает go vet; позиции
// ошибок переводятся в строки MDX.
func vetGo(t *testing.T, snips []docscheck.Snippet) error {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module docsnippets\n\ngo 1.27\n"), 0o600))
	for i, s := range snips {
		pkg := filepath.Join(dir, fmt.Sprintf("s%03d", i))
		require.NoError(t, os.Mkdir(pkg, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(pkg, "main.go"), []byte(s.Code), 0o600))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "vet", "./...")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=", "GOPROXY=off", "GOWORK=off", "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	return fmt.Errorf("go vet: %w\n%s", err, mdxPositions(string(out), snips))
}

var snippetPos = regexp.MustCompile(`(?:\./)?s(\d{3})[/\\]main\.go:(\d+)`)

// mdxPositions заменяет sNNN/main.go:L на файл MDX и строку: строка 1 сниппета идёт сразу за открывающим
// забором.
func mdxPositions(out string, snips []docscheck.Snippet) string {
	return snippetPos.ReplaceAllStringFunc(out, func(m string) string {
		sub := snippetPos.FindStringSubmatch(m)
		i, _ := strconv.Atoi(sub[1])
		line, _ := strconv.Atoi(sub[2])
		if i >= len(snips) {
			return m
		}
		return fmt.Sprintf("%s:%d", snips[i].File, snips[i].Line+line)
	})
}

// runShellSnippet выполняет shell-сниппет в bash -euo pipefail во временном каталоге: боевой адрес ingest
// заменён на base, ключ — в переменной AM_PK.
func runShellSnippet(t *testing.T, s docscheck.Snippet, base, key string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), snippetTimeout)
	defer cancel()
	code := strings.ReplaceAll(s.Code, prodIngestBase, base)
	cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", "-c", code)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "AM_PK="+key)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %w", s.Pos(), err)
	}
	return string(out), nil
}

// response — ответ ingest, который получил сниппет.
type response struct {
	status int
	body   []byte
}

// checkResponses требует хотя бы один запрос, и каждый — 200 с accepted > 0. Возвращает сумму accepted.
func checkResponses(reqs []response) (int, error) {
	if len(reqs) == 0 {
		return 0, errors.New("сниппет не отправил ни одного запроса в ingest")
	}
	total := 0
	var errs []error
	for i, r := range reqs {
		if r.status != http.StatusOK {
			errs = append(errs, fmt.Errorf("запрос %d: HTTP %d: %s", i+1, r.status, r.body))
			continue
		}
		var res struct {
			Accepted int `json:"accepted"`
		}
		if err := json.Unmarshal(r.body, &res); err != nil {
			errs = append(errs, fmt.Errorf("запрос %d: ответ не IngestResult: %w", i+1, err))
			continue
		}
		if res.Accepted == 0 {
			errs = append(errs, fmt.Errorf("запрос %d: accepted = 0: %s", i+1, r.body))
		}
		total += res.Accepted
	}
	return total, errors.Join(errs...)
}

// ingestTarget — локальный ingest с журналом ответов. Хендлер и проверку записанных событий собирает
// newIngest: in-process с ClickHouse в памяти или, под тегом integration, с настоящими Postgres и ClickHouse.
type ingestTarget struct {
	url    string
	verify func(t *testing.T, accepted int)

	mu   sync.Mutex
	log  []response
	seen int
}

func startIngest(t *testing.T) *ingestTarget {
	t.Helper()
	h, verify := newIngest(t)
	target := &ingestTarget{verify: verify}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if r.Method == http.MethodPost {
			target.mu.Lock()
			target.log = append(target.log, response{status: rec.Code, body: bytes.Clone(rec.Body.Bytes())})
			target.mu.Unlock()
		}
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	}))
	t.Cleanup(srv.Close)
	target.url = srv.URL
	return target
}

// take возвращает ответы с прошлого вызова.
func (it *ingestTarget) take() []response {
	it.mu.Lock()
	defer it.mu.Unlock()
	out := it.log[it.seen:]
	it.seen = len(it.log)
	return out
}
