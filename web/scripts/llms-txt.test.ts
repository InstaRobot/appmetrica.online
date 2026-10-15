import { existsSync, readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";

// /llms.txt — инструкция по интеграции для ИИ-агентов (web/public, отдаётся как есть). Факты в ней сверяются с
// контрактом ingest и документацией: новый рецепт, код отказа или значение перечисления без строки в llms.txt,
// битая ссылка или якорь ломают этот тест.
const WEB = path.resolve(import.meta.dirname, "..");
const DOCS = path.join(WEB, "content/docs");
const SITE = "https://appmetrica.online";
const LLMS = path.join(WEB, "public/llms.txt");
const llms = existsSync(LLMS) ? readFileSync(LLMS, "utf8") : "";
const contract = readFileSync(path.join(WEB, "../contracts/openapi/ingest.yaml"), "utf8");

function contractEnum(schema: string): string[] {
  const match = contract.match(
    new RegExp(
      `\\n {4}${schema}:\\n(?: {6}.*\\n)*? {6}enum:(?: \\[([^\\]]*)\\]|\\n((?: {8}- .*\\n)+))`,
    ),
  );
  if (!match) throw new Error(`enum ${schema} не найден в ingest.yaml`);
  const values =
    match[1] !== undefined
      ? match[1].split(",")
      : (match[2] ?? "").split("\n").map((l) => l.replace(/^\s*-\s*/, ""));
  return values.map((v) => v.trim()).filter(Boolean);
}

// Страница документации по пути после /docs: "" → index.mdx, "concepts/sessions" → concepts/sessions.mdx.
function docsPage(slug: string): string | null {
  const candidates = slug === "" ? ["index.mdx"] : [`${slug}.mdx`, `${slug}/index.mdx`];
  const file = candidates.map((c) => path.join(DOCS, c)).find((f) => existsSync(f));
  return file ? readFileSync(file, "utf8") : null;
}

const docsLinks = [
  ...llms.matchAll(/https:\/\/appmetrica\.online\/docs(?:\/([a-z0-9/-]+))?(?:#([a-z0-9-]+))?/g),
].map(([url, slug = "", anchor]) => ({ url, slug, anchor }));

describe("llms.txt", () => {
  it("exists in web/public", () => {
    expect(existsSync(LLMS)).toBe(true);
  });

  it("follows the llms.txt layout: a title and a summary", () => {
    expect(llms).toMatch(/^# appmetrica\.online\n\n> \S/);
  });

  it("is in English", () => {
    expect(llms).not.toMatch(/\p{Script=Cyrillic}/u);
  });

  it.each(
    readdirSync(path.join(DOCS, "recipes"))
      .filter((name) => name.endsWith(".mdx") && !name.endsWith(".ru.mdx"))
      .map((name) => name.replace(/\.mdx$/, "")),
  )("links the %s recipe", (slug) => {
    expect(llms).toContain(`${SITE}/docs/recipes/${slug}`);
  });

  it("links the full docs as Markdown", () => {
    expect(llms).toContain(`${SITE}/llms-full.txt`);
  });

  it("finds docs links", () => {
    expect(docsLinks.length).toBeGreaterThan(10);
  });

  it.each(docsLinks.filter((l) => !l.slug.startsWith("api-reference/")))(
    "$url points to an existing page and anchor",
    ({ slug, anchor }) => {
      const page = docsPage(slug);
      expect(page, `content/docs/${slug || "index"}.mdx`).not.toBeNull();
      if (anchor) expect(page).toMatch(new RegExp(`^#{1,6} .*\\[#${anchor}\\]\\s*$`, "m"));
    },
  );

  it.each(docsLinks.filter((l) => l.slug.startsWith("api-reference/")))(
    "$url is an API reference page the guides link to",
    ({ slug }) => {
      const guides = readdirSync(DOCS, { recursive: true, encoding: "utf8" })
        .filter((name) => name.endsWith(".mdx"))
        .map((name) => readFileSync(path.join(DOCS, name), "utf8"))
        .join("\n");
      expect(guides).toContain(`(/docs/${slug})`);
    },
  );

  it("uses the placeholder ingest address, not a real installation", () => {
    expect(llms).toContain("https://i.analytics.example.com/v1/events");
    expect(llms).not.toContain("i.appmetrica.online");
  });

  it.each(["Platform", "DeviceType", "Environment", "EventName", "RejectionCode", "WarningCode"])(
    "lists every %s value of the ingest contract",
    (schema) => {
      const values = contractEnum(schema);
      expect(values.length).toBeGreaterThan(1);
      for (const value of values) expect(llms).toContain(`\`${value}\``);
    },
  );

  it("states the batch limit of the contract", () => {
    const maxItems = contract.match(
      /events:\n\s+type: array\n\s+minItems: 1\n\s+maxItems: (\d+)/,
    )?.[1];
    expect(maxItems).toBe("500");
    expect(llms).toContain(`1 to ${maxItems} events`);
  });
});
