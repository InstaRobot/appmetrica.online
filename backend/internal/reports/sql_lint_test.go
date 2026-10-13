package reports

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// sqlSources — пакеты с SQL отчётов: SQL-фрагменты склейки (identity) и запросы reports.
var sqlSources = []string{".", filepath.Join("..", "identity")}

// peopleMetrics — псевдонимы колонок с числом людей или сессий. До слияния частей ReplacingMergeTree дубли
// событий видны, поэтому эти метрики считаются только uniqExact (05-data-model.md §2.7, ADR-007).
var peopleMetrics = map[string]bool{
	"active": true, "new": true, "sessions": true,
	"dau": true, "wau": true, "mau": true,
	"active_now": true, "active_today": true, "active_date": true, "active_same_day_prev_week": true,
	"new_date": true, "sessions_date": true, "sessions_started": true, "persons": true,
}

var (
	reAlias = regexp.MustCompile(`(?i)\bAS\s+([a-z_][a-z0-9_]*)`)
	// reCounting — агрегаты, считающие строки или приближённо уникальные: count, countIf, uniq, uniqCombined…
	// uniqExact* проверяется отдельно и разрешён.
	reCounting = regexp.MustCompile(`(?i)\b(count[a-z0-9]*|uniq[a-z0-9]*)\s*\(`)
	// reCountEvents — count() в одном фрагменте с чтением сырой таблицы events.
	reCountEvents = regexp.MustCompile(`(?is)\bcount(if)?\s*\(.*\bFROM\s+events\b|\bFROM\s+events\b.*\bcount(if)?\s*\(`)
)

// AC-027.6: в отчётных SQL нет count() (и приближённых uniq) в выражениях людей и сессий — только uniqExact,
// и нет count() по сырой events.
func TestSQL_NoCountForPeople(t *testing.T) {
	literals := reportSQLLiterals(t)
	var sql int
	for _, l := range literals {
		if !strings.Contains(l.text, "SELECT") && !strings.Contains(l.text, "FROM") {
			continue
		}
		sql++
		for _, v := range lintReportSQL(l.text) {
			t.Errorf("%s: %s", l.pos, v)
		}
	}
	require.Positive(t, sql, "линтер не нашёл ни одного SQL-литерала в %v", sqlSources)
}

func TestLintReportSQL(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		bad  bool
	}{
		{"uniq_exact_active", `SELECT uniqExact(person_key) AS active FROM ev`, false},
		{"uniq_exact_if_sessions", `SELECT uniqExactIf(session_id, name = 'session_start') AS sessions FROM ev`, false},
		{"merge_window", `SELECT uniqExactMerge(st) OVER (ORDER BY day RANGE BETWEEN 6 PRECEDING AND CURRENT ROW) AS wau FROM d`, false},
		{"duration_avg", `SELECT if(count() = 0, NULL, avg(duration_ms)) AS avg_duration_ms FROM sess`, false},
		{"count_of_other_metric", `SELECT code, count() AS rejected FROM rejected_events GROUP BY code`, false},
		{"lowercase_as", `select uniqExact(person_key) as active from ev`, false},
		{"several_items", `SELECT uniqExact(person_key) AS active, uniqExactIf(session_id, name = 'session_start') AS sessions FROM ev`, false},

		{"count_active", `SELECT count() AS active FROM active_persons`, true},
		{"count_distinct_active", `SELECT count(DISTINCT person_key) AS active FROM ev`, true},
		{"countif_sessions", `SELECT countIf(name = 'session_start') AS sessions FROM ev`, true},
		{"uniq_approx_new", `SELECT uniq(person_key) AS new FROM ev`, true},
		{"uniq_combined_dau", `SELECT uniqCombined(person_key) AS dau FROM ev`, true},
		{"subquery_count", `SELECT (SELECT count() FROM active_persons) AS active`, true},
		{"second_item", `SELECT uniqExact(person_key) AS active, count() AS sessions FROM ev`, true},
		{"lowercase_count", `select count(*) as persons from cohort`, true},
		{"count_raw_events", `SELECT count() AS accepted FROM events WHERE app_id = {app_id:UUID}`, true},
		{"countif_raw_events", `SELECT countIf(name = 'session_end') AS ends FROM events AS e`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := lintReportSQL(tc.sql)
			if tc.bad {
				require.NotEmpty(t, got, "линтер пропустил нарушение")
			} else {
				require.Empty(t, got)
			}
		})
	}
}

// lintReportSQL возвращает нарушения правил подсчёта людей и сессий в одном SQL-литерале.
func lintReportSQL(sql string) []string {
	var out []string
	for _, m := range reAlias.FindAllStringSubmatchIndex(sql, -1) {
		alias := strings.ToLower(sql[m[2]:m[3]])
		if !peopleMetrics[alias] {
			continue
		}
		expr := selectItemBefore(sql, m[0])
		for _, c := range reCounting.FindAllStringSubmatch(expr, -1) {
			if !strings.HasPrefix(strings.ToLower(c[1]), "uniqexact") {
				out = append(out, alias+": "+c[1]+"() вместо uniqExact в «"+strings.TrimSpace(expr)+"»")
			}
		}
	}
	if reCountEvents.MatchString(sql) {
		out = append(out, "count() по сырой таблице events: дубли до слияния частей завышают число")
	}
	return out
}

// selectItemBefore — выражение элемента SELECT, который заканчивается на позиции end (перед AS):
// назад до запятой или ключевого слова SELECT на том же уровне скобок либо до открывающей скобки уровня выше.
func selectItemBefore(sql string, end int) string {
	depth := 0
	for i := end - 1; i >= 0; i-- {
		switch sql[i] {
		case ')':
			depth++
		case '(':
			if depth == 0 {
				return sql[i+1 : end]
			}
			depth--
		case ',':
			if depth == 0 {
				return sql[i+1 : end]
			}
		}
		if depth == 0 && i >= 6 && strings.EqualFold(sql[i-6:i], "SELECT") {
			return sql[i:end]
		}
	}
	return sql[:end]
}

type sqlLiteral struct {
	pos  string
	text string
}

// reportSQLLiterals — строковые литералы не тестовых файлов sqlSources (SQL отчётов — Go-константы,
// склеенные из литералов; каждый литерал проверяется отдельно).
func reportSQLLiterals(t *testing.T) []sqlLiteral {
	t.Helper()
	fset := token.NewFileSet()
	var out []sqlLiteral
	for _, dir := range sqlSources {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		require.NoError(t, err)
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			require.NoError(t, err)
			ast.Inspect(f, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				out = append(out, sqlLiteral{pos: fset.Position(lit.Pos()).String(), text: s})
				return true
			})
		}
	}
	return out
}
