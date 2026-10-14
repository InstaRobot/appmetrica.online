// @vitest-environment node
import type { Document } from "fumadocs-openapi";
import { describe, expect, it } from "vitest";
import { createDocsOpenAPI, openApiSource, publicDocument } from "./openapi-source";

type Operation = { schema: string; method: string; path: string; internal: boolean };

async function generated() {
  const { files } = await openApiSource(createDocsOpenAPI());
  const pages = files.filter((f) => f.type === "page");
  const operations: Operation[] = pages.flatMap((page) => {
    const { id, bundled } = page.data.getSchema();
    return (page.data.getOpenAPIPageProps().operations ?? []).map(({ path, method }) => {
      const op = bundled.paths?.[path]?.[method] as Record<string, unknown> | undefined;
      return {
        schema: id,
        method: method.toUpperCase(),
        path,
        internal: op?.["x-internal"] === true,
      };
    });
  });
  return { files, pages, operations };
}

describe("openApiSource", () => {
  it("excludes x-internal operations", async () => {
    const { pages, operations } = await generated();

    expect(operations).toContainEqual({
      schema: "ingest",
      method: "POST",
      path: "/v1/events",
      internal: false,
    });
    expect(operations).toContainEqual({
      schema: "read",
      method: "GET",
      path: "/v1/portfolio",
      internal: false,
    });
    expect(operations.filter((op) => op.internal)).toEqual([]);
    expect(operations.filter((op) => !op.path.startsWith("/v1/"))).toEqual([]);

    // В клиентский payload страницы не уходит ни одна операция /admin/v1.
    for (const page of pages) {
      const { bundled } = page.data.getSchema();
      expect(Object.keys(bundled.paths ?? {}).filter((p) => !p.startsWith("/v1/"))).toEqual([]);
    }
  });

  it("puts ingest and Read API operations into separate sections", async () => {
    const { files } = await generated();
    const paths = files.map((f) => f.path);

    expect(paths).toContain("api-reference/ingest/ingest-events.mdx");
    expect(paths).toContain("api-reference/read/get-portfolio.mdx");
    expect(paths).not.toContain("api-reference/read/get-auth-state.mdx");

    const titles = Object.fromEntries(
      files.flatMap((f) => (f.type === "meta" ? [[f.path, f.data.title]] : [])),
    );
    expect(titles).toMatchObject({
      "api-reference/meta.json": "API reference",
      "api-reference/ingest/meta.json": "Ingestion API",
      "api-reference/read/meta.json": "Read API",
    });
  });

  it("skips CORS preflight and health endpoints", async () => {
    const { operations } = await generated();
    const ids = operations.map((op) => `${op.method} ${op.path}`);

    expect(ids).not.toContain("OPTIONS /v1/events");
    expect(operations.some((op) => op.path === "/health" || op.path === "/ready")).toBe(false);
  });
});

const ok = { responses: { "200": { description: "ok" } } };

function sample(): Document {
  return {
    openapi: "3.0.3",
    info: { title: "t", version: "1" },
    tags: [{ name: "Apps" }, { name: "Admin Apps", "x-internal": true }],
    paths: {
      "/v1/apps": {
        get: { ...ok, tags: ["Apps"] },
        post: { ...ok, tags: ["Apps"], "x-internal": true },
      },
      "/v1/hidden": { "x-internal": true, get: ok },
      "/v1/mixed": { get: { ...ok, tags: ["Admin Apps", "Apps"] } },
      "/v1/admin-only": { get: { ...ok, tags: ["Admin Apps"] } },
      "/admin/v1/apps": { get: { ...ok, tags: ["Apps"] } },
      "/health": { get: ok },
    },
  } as Document;
}

describe("publicDocument", () => {
  it("keeps public siblings of an internal operation", () => {
    const doc = publicDocument(sample());

    expect(Object.keys(doc.paths ?? {})).toEqual(["/v1/apps", "/v1/mixed"]);
    expect(doc.paths?.["/v1/apps"]).toHaveProperty("get");
    expect(doc.paths?.["/v1/apps"]).not.toHaveProperty("post");
  });

  it("drops internal path items, internal-only tags and non-/v1 routes", () => {
    const doc = publicDocument(sample());

    expect(doc.paths).not.toHaveProperty("/v1/hidden");
    expect(doc.paths).not.toHaveProperty("/v1/admin-only");
    expect(doc.paths).not.toHaveProperty("/admin/v1/apps");
    expect(doc.paths).not.toHaveProperty("/health");
    expect(doc.tags).toEqual([{ name: "Apps" }]);
  });

  it("does not mutate the input document", () => {
    const input = sample();
    const before = structuredClone(input);

    publicDocument(input);

    expect(input).toEqual(before);
  });

  it("handles a document without paths and tags", () => {
    const doc = publicDocument({
      openapi: "3.0.3",
      info: { title: "t", version: "1" },
    } as Document);

    expect(doc.paths).toEqual({});
    expect(doc).not.toHaveProperty("tags");
  });
});
