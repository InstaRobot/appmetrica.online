package docscheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/docscheck"
)

func TestExtract(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []docscheck.Snippet
	}{
		{
			name: "language, meta and line of the opening fence",
			src: "---\ntitle: T\n---\n\nText.\n\n```go title=\"main.go\" tab=\"Go\"\npackage main\n\nfunc main() {}\n```\n",
			want: []docscheck.Snippet{{
				File: "page.mdx", Line: 7, Lang: "go", Meta: `title="main.go" tab="Go"`,
				Code: "package main\n\nfunc main() {}\n",
			}},
		},
		{
			name: "several blocks keep document order",
			src:  "```bash\necho 1\n```\n\n```swift tab=\"SwiftUI\"\nlet a = 1\n```\n```kotlin\nval b = 2\n```\n",
			want: []docscheck.Snippet{
				{File: "page.mdx", Line: 1, Lang: "bash", Code: "echo 1\n"},
				{File: "page.mdx", Line: 5, Lang: "swift", Meta: `tab="SwiftUI"`, Code: "let a = 1\n"},
				{File: "page.mdx", Line: 8, Lang: "kotlin", Code: "val b = 2\n"},
			},
		},
		{
			name: "longer fence contains a shorter one",
			src:  "````md\n```go\npackage main\n```\n````\n",
			want: []docscheck.Snippet{{File: "page.mdx", Line: 1, Lang: "md", Code: "```go\npackage main\n```\n"}},
		},
		{
			name: "closing fence may be longer, shorter one is content",
			src:  "```bash\na\n``\n`````\n",
			want: []docscheck.Snippet{{File: "page.mdx", Line: 1, Lang: "bash", Code: "a\n``\n"}},
		},
		{
			name: "tilde fence is not closed by backticks",
			src:  "~~~go\npackage main\n```\n~~~\n",
			want: []docscheck.Snippet{{File: "page.mdx", Line: 1, Lang: "go", Code: "package main\n```\n"}},
		},
		{
			name: "fence with info is not a closing fence",
			src:  "```bash\n```go\n```\n",
			want: []docscheck.Snippet{{File: "page.mdx", Line: 1, Lang: "bash", Code: "```go\n"}},
		},
		{
			name: "indented fence inside a list item strips its indentation",
			src:  "1. Step:\n\n   ```bash\n   curl -sS \\\n     -X POST\n   ```\n",
			want: []docscheck.Snippet{{File: "page.mdx", Line: 3, Lang: "bash", Code: "curl -sS \\\n  -X POST\n"}},
		},
		{
			name: "block without language and empty block",
			src:  "```\nplain\n```\n\n```go\n```\n",
			want: []docscheck.Snippet{
				{File: "page.mdx", Line: 1, Code: "plain\n"},
				{File: "page.mdx", Line: 5, Lang: "go"},
			},
		},
		{
			name: "CRLF line endings",
			src:  "```go\r\npackage main\r\n```\r\n",
			want: []docscheck.Snippet{{File: "page.mdx", Line: 1, Lang: "go", Code: "package main\n"}},
		},
		{
			name: "inline code and two backticks are not fences",
			src:  "Use `go vet` and ``a`` here.\n",
		},
		{
			name: "backtick fence with backtick in info is not a fence",
			src:  "```go `x`\nnot code\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := docscheck.Extract("page.mdx", []byte(tt.src))
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestExtract_UnclosedFence(t *testing.T) {
	_, err := docscheck.Extract("recipes/x.mdx", []byte("Text\n\n```go\npackage main\n"))
	require.EqualError(t, err, "recipes/x.mdx:3: незакрытый блок кода ```go")
}

func TestExtractDir(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	write("recipes/b.mdx", "```go\nb\n```\n")
	write("index.mdx", "```bash\na\n```\n")
	write("concepts/c.mdx", "no code\n")
	write("meta.json", "```go\nnot mdx\n```\n")

	got, err := docscheck.ExtractDir(root)
	require.NoError(t, err)
	require.Equal(t, []docscheck.Snippet{
		{File: "index.mdx", Line: 1, Lang: "bash", Code: "a\n"},
		{File: "recipes/b.mdx", Line: 1, Lang: "go", Code: "b\n"},
	}, got)

	require.Equal(t, []docscheck.Snippet{got[1]}, docscheck.ByLang(got, "go"))
	require.Equal(t, []docscheck.Snippet{got[0]}, docscheck.ByLang(got, "sh", "bash"))
	require.Empty(t, docscheck.ByLang(got, "kotlin"))
	require.Equal(t, "recipes/b.mdx:1", got[1].Pos())
}

func TestExtractDir_Errors(t *testing.T) {
	_, err := docscheck.ExtractDir(filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "bad.mdx"), []byte("~~~bash\necho\n"), 0o600))
	_, err = docscheck.ExtractDir(root)
	require.EqualError(t, err, "bad.mdx:1: незакрытый блок кода ~~~bash")
}
