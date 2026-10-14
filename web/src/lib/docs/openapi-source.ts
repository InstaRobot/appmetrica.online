import path from "node:path";
import type { Document, OperationOutput, PagesBuilder, WebhookOutput } from "fumadocs-openapi";
import { createOpenAPI, type OpenAPIServer, openapiSource } from "fumadocs-openapi/server";

// API reference документации (S-13, FR-023): страницы операций из contracts/openapi/{ingest,api}.yaml.
// Спеки читаются из копий в gen/openapi (`make gen-web`): образ web собирается без contracts/.
// В справочник попадают только публичные операции /v1: Admin API (/admin/v1, x-internal), health
// и CORS preflight клиенты по документации не вызывают.
// Пути — литералы: по ним сборка Next трассирует YAML в standalone-образ.
const SECTIONS = {
  ingest: {
    file: path.join(process.cwd(), "src/lib/api/gen/openapi/ingest.yaml"),
    title: "Ingestion API",
  },
  read: {
    file: path.join(process.cwd(), "src/lib/api/gen/openapi/api.yaml"),
    title: "Read API",
  },
} as const;

const BASE_DIR = "api-reference";
const METHODS = ["get", "put", "post", "delete", "patch", "head", "trace", "query"] as const;

function isInternal(value: unknown): boolean {
  return typeof value === "object" && value !== null && "x-internal" in value
    ? value["x-internal"] === true
    : false;
}

// publicDocument возвращает копию спеки только с публичными операциями /v1. Операция скрыта, если
// x-internal стоит на ней, на её path item или на всех её тегах.
export function publicDocument(doc: Document): Document {
  const internalTags = new Set(
    (doc.tags ?? []).filter((tag) => isInternal(tag)).map((tag) => tag.name),
  );
  const paths: NonNullable<Document["paths"]> = {};

  for (const [route, item] of Object.entries(doc.paths ?? {})) {
    if (!route.startsWith("/v1/") || !item || isInternal(item)) continue;
    const { options: _preflight, ...publicItem } = item;
    let operations = 0;
    for (const method of METHODS) {
      const operation = publicItem[method];
      if (!operation) continue;
      const tags = operation.tags ?? [];
      if (isInternal(operation) || (tags.length > 0 && tags.every((t) => internalTags.has(t)))) {
        delete publicItem[method];
      } else {
        operations++;
      }
    }
    if (operations > 0) paths[route] = publicItem;
  }

  const { tags, ...rest } = doc;
  return {
    ...rest,
    paths,
    ...(tags ? { tags: tags.filter((tag) => !isInternal(tag)) } : {}),
  } as Document;
}

// createDocsOpenAPI загружает спеки (YAML → bundle, OpenAPI 3.0 → 3.2) и отдаёт Fumadocs уже
// отфильтрованные документы: в клиентский payload страницы Admin API тоже не попадает.
export function createDocsOpenAPI(): OpenAPIServer {
  const raw = createOpenAPI({ input: Object.values(SECTIONS).map((s) => s.file) });
  const input = Object.fromEntries(
    Object.entries(SECTIONS).map(([id, s]) => [
      id,
      async () => publicDocument((await raw.getSchema(s.file)).bundled),
    ]),
  );
  return createOpenAPI({ input });
}

// ingestEvents → ingest-events: адреса страниц в нижнем регистре.
function operationFileName(
  this: PagesBuilder,
  output: Omit<OperationOutput, "path"> | Omit<WebhookOutput, "path">,
): string {
  const item = output.item;
  const operation = "path" in item ? this.document.paths?.[item.path]?.[item.method] : undefined;
  const name =
    operation?.operationId ?? ("name" in item ? item.name : `${item.method}-${item.path}`);
  return name
    .replace(/([a-z0-9])([A-Z])/g, "$1-$2")
    .replace(/[^A-Za-z0-9]+/g, "-")
    .replace(/^-|-$/g, "")
    .toLowerCase();
}

// openApiSource — виртуальные страницы для loader() Fumadocs: api-reference/ingest/* и
// api-reference/read/*, порядок страниц — как в спеке.
export async function openApiSource(
  server: OpenAPIServer = createDocsOpenAPI(),
): ReturnType<typeof openapiSource> {
  const source = await openapiSource(server, {
    baseDir: BASE_DIR,
    groupBy: (entry) => entry.schemaId,
    name: operationFileName,
    meta: true,
  });
  const rootMeta = `${BASE_DIR}/meta.json`;
  const files: typeof source.files = [];
  for (const file of source.files) {
    // Fumadocs пишет api-reference/meta.json для каждой спеки отдельно, и последняя затирает
    // остальные: корневой meta собираем сами.
    if (file.type === "meta" && file.path === rootMeta) continue;
    const id = path.basename(path.dirname(file.path));
    if (file.type === "meta" && id in SECTIONS) {
      files.push({
        ...file,
        data: { ...file.data, title: SECTIONS[id as keyof typeof SECTIONS].title },
      });
    } else {
      files.push(file);
    }
  }
  files.push({
    type: "meta",
    path: rootMeta,
    data: { title: "API reference", pages: Object.keys(SECTIONS), defaultOpen: true },
  });
  return { ...source, files };
}
