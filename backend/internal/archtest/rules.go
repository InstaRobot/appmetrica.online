// Package archtest проверяет правила зависимостей между Go-пакетами из 07-project-structure.md §5.
// Тест arch_test.go запускается в обычном `go test ./...` и падает при запрещённом импорте.
package archtest

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

const module = "appmetrica.online/backend/"

// Graph — прямые (не тестовые) импорты: полный путь пакета → полные пути импортируемых.
type Graph map[string][]string

// free — пакеты без ограничений: точка сборки, тестовая инфраструктура и сгенерированный код.
const free = "*"

// rules — какие пакеты internal/* может импортировать пакет (пути относительно модуля).
// Собственные подпакеты разрешены всегда; разрешение пакета распространяется на его подпакеты
// (internal/pgstore разрешает и internal/pgstore/sqlc); импорты вне internal/ не проверяются.
var rules = map[string][]string{
	"cmd/appmetrica": {free},
	"internal/api": {
		"internal/gen/api", "internal/auth", "internal/apps", "internal/reports", "internal/deletion", "internal/merge",
		"internal/identity", "internal/pgstore", "internal/chstore", "internal/httpx", "internal/clock", "internal/config",
	},
	"internal/ingest": {
		"internal/gen/ingestapi", "internal/geo", "internal/ratelimit", "internal/chstore", "internal/pgstore",
		"internal/httpx", "internal/clock", "internal/config",
	},
	"internal/worker": {
		"internal/reports", "internal/deletion", "internal/merge", "internal/telegram", "internal/identity",
		"internal/chstore", "internal/pgstore", "internal/clock", "internal/config",
	},
	"internal/reports":  {"internal/chstore", "internal/identity", "internal/clock"},
	"internal/deletion": {"internal/chstore", "internal/pgstore", "internal/identity", "internal/clock"},
	"internal/merge": {
		"internal/pgstore", "internal/chstore", "internal/deletion", "internal/identity", "internal/clock", "internal/config",
	},
	"internal/auth":      {"internal/pgstore", "internal/clock", "internal/config"},
	"internal/apps":      {"internal/pgstore", "internal/clock", "internal/config"},
	"internal/identity":  {},
	"internal/clock":     {},
	"internal/geo":       {},
	"internal/ratelimit": {},
	"internal/telegram":  {},
	"internal/chstore":   {"internal/clock"},
	"internal/pgstore":   {"internal/clock"},
	"internal/httpx":     {"internal/clock"},
	"internal/config":    {"internal/clock"},

	"internal/testutil":      {free},
	"internal/archtest":      {free},
	"internal/docscheck":     {free},
	"internal/gen/api":       {free},
	"internal/gen/ingestapi": {free},
	"internal/pgstore/sqlc":  {free},
}

// Check возвращает отсортированный список нарушений вида "internal/ingest → internal/api".
func Check(g Graph) []string {
	var out []string
	for pkg, imports := range g {
		rel, ok := strings.CutPrefix(pkg, module)
		if !ok {
			continue
		}
		allowed, known := rules[rel]
		if !known {
			if strings.HasPrefix(rel, "internal/") {
				out = append(out, rel+": нет правила в archtest (07-project-structure.md §5)")
			}
			continue
		}
		if slices.Contains(allowed, free) {
			continue
		}
		for _, imp := range imports {
			dep, ok := strings.CutPrefix(imp, module)
			if !ok || !strings.HasPrefix(dep, "internal/") || strings.HasPrefix(dep, rel+"/") {
				continue
			}
			if !slices.ContainsFunc(allowed, func(a string) bool { return dep == a || strings.HasPrefix(dep, a+"/") }) {
				out = append(out, rel+" → "+dep)
			}
		}
	}
	slices.Sort(out)
	return out
}

// LoadGraph строит граф импортов всех пакетов модуля через `go list` (рабочий каталог — внутри модуля).
func LoadGraph(ctx context.Context) (Graph, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-f", `{{.ImportPath}}{{range .Imports}} {{.}}{{end}}`, module+"...")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("archtest: go list: %w: %s", err, stderr.String())
	}
	g := Graph{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) > 0 {
			g[fields[0]] = fields[1:]
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("archtest: read go list output: %w", err)
	}
	return g, nil
}
