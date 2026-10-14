// Package docscheck извлекает блоки кода из MDX-документации (web/content/docs), чтобы тесты проверяли
// сниппеты: Go — go vet, curl — запросом в локальный ingest (FR-023, T-048).
package docscheck

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Snippet — огороженный блок кода MDX.
type Snippet struct {
	File string // путь MDX относительно корня документации, через /
	Line int    // строка открывающего забора, с 1; первая строка кода — Line+1
	Lang string // первое слово info string (```go → go)
	Meta string // остаток info string: title="…" tab="…"
	Code string // содержимое без заборов, каждая строка с \n
}

// Pos — «файл:строка» открывающего забора для сообщений об ошибках.
func (s Snippet) Pos() string { return s.File + ":" + strconv.Itoa(s.Line) }

// fence — открытый блок: символ и длина забора, отступ открывающей строки.
type fence struct {
	char   byte
	n      int
	indent int
}

// Extract возвращает блоки кода документа в порядке появления. Правила заборов — CommonMark: три и больше
// ` или ~, закрывающий забор из того же символа не короче открывающего и без info string, info string
// забора из ` не содержит `. Отступ открывающего забора снимается со строк кода: MDX не знает отступных
// блоков кода, поэтому заборы внутри списков и JSX бывают с любым отступом. Незакрытый блок — ошибка:
// в документации это опечатка, а не код до конца файла.
func Extract(file string, src []byte) ([]Snippet, error) {
	var (
		out  []Snippet
		open *fence
		cur  Snippet
		code strings.Builder
	)
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	for i, line := range lines {
		if open == nil {
			f, info, ok := openingFence(line)
			if !ok {
				continue
			}
			open = &f
			lang, meta, _ := strings.Cut(info, " ")
			cur = Snippet{File: file, Line: i + 1, Lang: lang, Meta: strings.TrimSpace(meta)}
			code.Reset()
			continue
		}
		if closesFence(line, *open) {
			cur.Code = code.String()
			out = append(out, cur)
			open = nil
			continue
		}
		code.WriteString(dedent(line, open.indent))
		code.WriteByte('\n')
	}
	if open != nil {
		return nil, fmt.Errorf("%s: незакрытый блок кода %s%s", cur.Pos(), strings.Repeat(string(open.char), open.n), cur.Lang)
	}
	return out, nil
}

// openingFence разбирает строку «отступ + забор + info string».
func openingFence(line string) (fence, string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return fence{}, "", false
	}
	f := fence{char: trimmed[0], n: countPrefix(trimmed, trimmed[0]), indent: len(line) - len(trimmed)}
	if f.n < 3 {
		return fence{}, "", false
	}
	info := strings.TrimSpace(trimmed[f.n:])
	if f.char == '`' && strings.Contains(info, "`") {
		return fence{}, "", false
	}
	return f, info, true
}

func closesFence(line string, f fence) bool {
	trimmed := strings.TrimLeft(line, " \t")
	n := countPrefix(trimmed, f.char)
	return n >= f.n && strings.TrimSpace(trimmed[n:]) == ""
}

func countPrefix(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// dedent снимает с начала строки не больше n пробельных символов.
func dedent(line string, n int) string {
	i := 0
	for i < n && i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[i:]
}

// ExtractDir извлекает блоки из всех *.mdx под root; File — путь относительно root через /, файлы — в
// лексикографическом порядке путей.
func ExtractDir(root string) ([]Snippet, error) {
	fsys := os.DirFS(root)
	var files []string
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".mdx") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	slices.Sort(files)

	var out []Snippet
	for _, file := range files {
		src, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		snips, err := Extract(file, bytes.TrimPrefix(src, []byte("\xef\xbb\xbf")))
		if err != nil {
			return nil, err
		}
		out = append(out, snips...)
	}
	return out, nil
}

// ByLang оставляет блоки с одним из языков.
func ByLang(snips []Snippet, langs ...string) []Snippet {
	var out []Snippet
	for _, s := range snips {
		if slices.Contains(langs, s.Lang) {
			out = append(out, s)
		}
	}
	return out
}
