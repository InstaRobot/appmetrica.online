package docscheck_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/testutil"
)

// codeRow — строка таблицы, первая колонка которой — код в обратных кавычках: | `platform_missing` | … |.
var codeRow = regexp.MustCompile("^\\|\\s*`([a-z0-9_]+)`\\s*\\|")

// Каждый код отказа и предупреждения из ingest.yaml описан в таблице своего раздела errors-and-retries.mdx:
// новый код в контракте без строки в документации ломает этот тест.
func TestDocsListIngestCodes(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join(testutil.BackendDir(), "..", "contracts", "openapi", "ingest.yaml"))
	require.NoError(t, err)
	src, err := os.ReadFile(filepath.Join(docsDir(), "errors-and-retries.mdx"))
	require.NoError(t, err)

	for schema, anchor := range map[string]string{"RejectionCode": "rejections", "WarningCode": "warnings"} {
		ref := doc.Components.Schemas[schema]
		require.NotNil(t, ref, schema)
		var want []string
		for _, v := range ref.Value.Enum {
			want = append(want, v.(string))
		}
		require.NotEmpty(t, want, schema)
		require.ElementsMatch(t, want, sectionCodes(string(src), anchor), "%s и таблица раздела #%s", schema, anchor)
	}
}

// sectionCodes — коды из первой колонки таблиц раздела с якорем anchor (до следующего заголовка того же или
// более высокого уровня); блоки кода пропускаются.
func sectionCodes(src, anchor string) []string {
	var codes []string
	level := 0
	inFence := false
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := headingAnchor.FindStringSubmatch(trimmed); m != nil || strings.HasPrefix(trimmed, "#") {
			hashes := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
			switch {
			case m != nil && m[1] == anchor:
				level = hashes
			case level > 0 && hashes <= level:
				return codes
			}
			continue
		}
		if level > 0 {
			if m := codeRow.FindStringSubmatch(trimmed); m != nil {
				codes = append(codes, m[1])
			}
		}
	}
	return codes
}

func TestSectionCodes(t *testing.T) {
	src := "## Отказы [#rejections]\n\n| Код | Причина |\n|---|---|\n| `missing_field` | Нет поля |\n\n" +
		"### Подробнее\n\n| `invalid_field` | Формат |\n\n```md\n| `in_fence` | Блок кода |\n```\n\n" +
		"## Предупреждения [#warnings]\n\n| `geo_ip_invalid` | IP |\nТекст с `inline_code` не строка таблицы.\n\n## Дальше\n\n| `other` | x |\n"
	require.Equal(t, []string{"missing_field", "invalid_field"}, sectionCodes(src, "rejections"))
	require.Equal(t, []string{"geo_ip_invalid"}, sectionCodes(src, "warnings"))
	require.Empty(t, sectionCodes(src, "nope"))
}
