package docscheck_test

import (
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	// docsLink — ссылка Markdown на страницу документации или на якорь текущей страницы:
	// ](/docs/recipes/curl#retry), ](/docs), ](#retry).
	docsLink = regexp.MustCompile(`\]\(((?:/docs[^)#\s]*)?)(#[^)\s]*)?\)`)
	// headingAnchor — явный якорь заголовка Fumadocs: ## Повторы [#retry].
	headingAnchor = regexp.MustCompile(`^#{1,6} .*\[#([A-Za-z0-9_-]+)\]\s*$`)
)

// docPage — страница MDX: якоря заголовков и ссылки вне блоков кода.
type docPage struct {
	file    string
	anchors map[string]bool
	links   [][2]string // путь /docs/… (пустой — та же страница) и якорь без #
}

// Ссылки гайдов ведут на существующие страницы и заголовки: переименование страницы или якоря без правки
// ссылок ломает этот тест, а не читателя. API reference генерируется из OpenAPI и проверяется
// openapi-source.test.ts, поэтому ссылки /docs/api-reference/… здесь пропускаются.
func TestDocsInternalLinks(t *testing.T) {
	pages := docPages(t)
	require.Contains(t, pages, "", "быстрый старт index.mdx")

	var broken []string
	for _, slug := range slices.Sorted(maps.Keys(pages)) {
		p := pages[slug]
		for _, l := range p.links {
			path, anchor := l[0], l[1]
			target := p
			if path != "" {
				if strings.HasPrefix(path, "/docs/api-reference/") {
					continue
				}
				var ok bool
				target, ok = pages[strings.Trim(strings.TrimPrefix(path, "/docs"), "/")]
				if !ok {
					broken = append(broken, p.file+": нет страницы "+path)
					continue
				}
			}
			if anchor != "" && !target.anchors[anchor] {
				broken = append(broken, p.file+": нет якоря #"+anchor+" в "+target.file)
			}
		}
	}
	require.Empty(t, broken)
}

// docPages читает MDX документации; ключ — путь страницы после /docs/ (index.mdx — пустая строка).
func docPages(t *testing.T) map[string]*docPage {
	t.Helper()
	fsys := os.DirFS(docsDir())
	pages := map[string]*docPage{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".mdx") {
			return err
		}
		src, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		slug := strings.TrimSuffix(path, ".mdx")
		if slug == "index" || strings.HasSuffix(slug, "/index") {
			slug = strings.TrimSuffix(strings.TrimSuffix(slug, "index"), "/")
		}
		pages[slug] = parsePage(path, string(src))
		return nil
	})
	require.NoError(t, err)
	return pages
}

func parsePage(file, src string) *docPage {
	p := &docPage{file: file, anchors: map[string]bool{}}
	var fence string
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(trimmed, fence) && strings.Trim(trimmed, fence[:1]) == "" {
				fence = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence = trimmed[:len(trimmed)-len(strings.TrimLeft(trimmed, trimmed[:1]))]
			continue
		}
		if m := headingAnchor.FindStringSubmatch(trimmed); m != nil {
			p.anchors[m[1]] = true
		}
		for _, m := range docsLink.FindAllStringSubmatch(line, -1) {
			if m[1] == "" && m[2] == "" {
				continue
			}
			p.links = append(p.links, [2]string{m[1], strings.TrimPrefix(m[2], "#")})
		}
	}
	return p
}

func TestParsePage(t *testing.T) {
	src := "---\ntitle: T\n---\n\n## Повторы [#retry]\n\nСм. [curl](/docs/recipes/curl#gzip), [выше](#retry) и\n" +
		"[DB-IP](https://db-ip.com), [старт](/docs).\n\n```bash\n# [не ссылка](/docs/nope)\n## Не заголовок [#nope]\n```\n\n" +
		"````md\n```\n[в блоке](/docs/nope)\n```\n````\n\n### Ещё [#more]\n"
	p := parsePage("page.mdx", src)
	require.Equal(t, map[string]bool{"retry": true, "more": true}, p.anchors)
	require.Equal(t, [][2]string{{"/docs/recipes/curl", "gzip"}, {"", "retry"}, {"/docs", ""}}, p.links)
}
