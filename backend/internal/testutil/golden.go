// Package testutil — тестовая инфраструктура backend: контейнеры ClickHouse и Postgres на пакет,
// изолированные базы на тест, NDJSON-фикстуры событий и golden-файлы ответов.
// Импортируется только из *_test.go и генератора testdata/gen.
package testutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "перезаписать *.golden.json фактическими ответами")

// BackendDir — абсолютный путь к каталогу модуля backend (не зависит от каталога запуска теста).
func BackendDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("testutil: runtime.Caller не вернул путь к golden.go")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// FixturesDir — backend/testdata/fixtures: NDJSON-фикстуры и *.golden.json.
func FixturesDir() string {
	return filepath.Join(BackendDir(), "testdata", "fixtures")
}

// Golden сравнивает ответы с эталонными JSON-файлами.
type Golden struct {
	Dir    string // каталог эталонов
	Update bool   // перезаписать эталон вместо сравнения
}

// AssertGolden сравнивает got с FixturesDir()/name; флаг -update перезаписывает эталон.
func AssertGolden(t testing.TB, name string, got any) {
	t.Helper()
	Golden{Dir: FixturesDir(), Update: *update}.Assert(t, name, got)
}

// Assert приводит got к каноническому JSON (ключи отсортированы, отступ 2 пробела, \n в конце) и сравнивает
// с эталоном. []byte и json.RawMessage считаются готовым JSON. Расхождение печатается unified diff.
func (g Golden) Assert(t testing.TB, name string, got any) {
	t.Helper()
	actual, err := canonicalJSON(got)
	require.NoError(t, err, "golden %s: ответ не сериализуется в JSON", name)

	path := filepath.Join(g.Dir, name)
	if g.Update {
		require.NoError(t, os.MkdirAll(g.Dir, 0o750))
		require.NoError(t, os.WriteFile(path, actual, 0o600))
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // путь к эталону задаёт тест
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("golden %s не найден; создайте его запуском `go test -update`", path)
	}
	require.NoError(t, err)
	require.Equal(t, string(want), string(actual),
		"golden %s расходится с ответом; если изменение ожидаемо, перезапустите `go test -update`", name)
}

func canonicalJSON(v any) ([]byte, error) {
	var raw []byte
	switch b := v.(type) {
	case []byte:
		raw = b
	case json.RawMessage:
		raw = b
	default:
		var err error
		if raw, err = json.Marshal(v); err != nil {
			return nil, err
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(generic); err != nil {
		return nil, err
	}
	return []byte(strings.TrimRight(buf.String(), "\n") + "\n"), nil
}
