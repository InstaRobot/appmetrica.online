import { docs } from "fumadocs-mdx:collections/server";
import { loader, multiple } from "fumadocs-core/source";
import { openapiPlugin } from "fumadocs-openapi/server";
import { openApiSource } from "./openapi-source";

// Единое дерево документации S-13: MDX-гайды из content/docs и виртуальные страницы API reference.
export const source = loader(
  multiple({ docs: docs.toFumadocsSource(), openapi: await openApiSource() }),
  { baseUrl: "/docs", plugins: [openapiPlugin()] },
);
