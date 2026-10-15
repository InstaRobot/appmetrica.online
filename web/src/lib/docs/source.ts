import { docs } from "fumadocs-mdx:collections/server";
import { loader, multiple } from "fumadocs-core/source";
import { openapiPlugin } from "fumadocs-openapi/server";
import { i18n } from "./i18n";
import { openApiSource } from "./openapi-source";

// Единое дерево документации S-13 на два языка: MDX-гайды из content/docs и виртуальные страницы API
// reference (только английские, русская версия показывает их через fallback).
export const source = loader(
  multiple({ docs: docs.toFumadocsSource(), openapi: await openApiSource() }),
  { baseUrl: "/docs", i18n, plugins: [openapiPlugin()] },
);
