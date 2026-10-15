package docscheck_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/testutil"
)

// Таблица «устройство → platform, device_type» на странице быстрого старта (обе локали) берёт значения только из
// перечислений Platform и DeviceType контракта ingest: несуществующее значение (например, `tvos`) сервер отклонит
// с invalid_field, поэтому документация не должна его советовать.
func TestDocsDeviceFamiliesUseContractValues(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(testutil.BackendDir(), "..", "contracts", "openapi", "ingest.yaml"))
	require.NoError(t, err)
	enum := func(schema string) []string {
		var values []string
		for _, v := range doc.Components.Schemas[schema].Value.Enum {
			values = append(values, v.(string))
		}
		return values
	}
	allowed := map[string][]string{"platform": enum("Platform"), "device_type": enum("DeviceType")}

	for _, file := range []string{"index.mdx", "index.ru.mdx"} {
		src, err := os.ReadFile(filepath.Join(docsDir(), file))
		require.NoError(t, err)
		rows := sectionTable(string(src), "device-families")
		require.Greater(t, len(rows), 1, "%s: таблица раздела #device-families", file)
		for column, values := range allowed {
			idx := slices.IndexFunc(rows[0], func(cell string) bool { return strings.Trim(cell, "`") == column })
			require.GreaterOrEqual(t, idx, 0, "%s: колонка %s", file, column)
			for _, row := range rows[1:] {
				require.Contains(t, values, strings.Trim(row[idx], "`"), "%s: строка %v", file, row)
			}
		}
	}
}

// sectionTable — ячейки строк первой таблицы раздела с якорем anchor (заголовок и данные, без разделителя |---|).
func sectionTable(src, anchor string) [][]string {
	var rows [][]string
	inSection := false
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if m := headingAnchor.FindStringSubmatch(trimmed); m != nil {
			if inSection {
				break
			}
			inSection = m[1] == anchor
			continue
		}
		if !inSection {
			continue
		}
		if !strings.HasPrefix(trimmed, "|") {
			if len(rows) > 0 {
				break
			}
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		if strings.Trim(strings.Join(cells, ""), "-: ") == "" {
			continue
		}
		rows = append(rows, cells)
	}
	return rows
}

func TestSectionTable(t *testing.T) {
	src := "## Платформа [#platform]\n\n| a | b |\n|---|---|\n| x | y |\n\n" +
		"### Семейства [#device-families]\n\nТекст.\n\n| Устройство | `platform` |\n|---|:---:|\n| Apple TV | `ios` |\n" +
		"| Android TV | `android` |\n\nПосле таблицы.\n\n| z | w |\n\n## Дальше [#next]\n"
	require.Equal(t, [][]string{{"Устройство", "`platform`"}, {"Apple TV", "`ios`"}, {"Android TV", "`android`"}},
		sectionTable(src, "device-families"))
	require.Empty(t, sectionTable(src, "nope"))
}
