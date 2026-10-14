import { defineConfig, defineDocs } from "fumadocs-mdx/config";

// Документация API (S-13): MDX-гайды в content/docs, страницы API reference генерирует fumadocs-openapi.
export const docs = defineDocs({
  dir: "content/docs",
});

// Подсветка кода — github-light-default, как у CodeBlock дашборда: красный github-light (#d73a49)
// не держит контраст 4.5:1 на фоне блока кода (axe color-contrast).
export default defineConfig({
  mdxOptions: {
    rehypeCodeOptions: {
      themes: { light: "github-light-default", dark: "github-dark-default" },
      defaultColor: false,
    },
  },
});
