import { defineConfig, defineDocs } from "fumadocs-mdx/config";

// Документация API (S-13): MDX-гайды в content/docs, страницы API reference генерирует fumadocs-openapi.
export const docs = defineDocs({
  dir: "content/docs",
});

export default defineConfig();
