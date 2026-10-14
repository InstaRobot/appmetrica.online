import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  buildSnippet,
  defaultSnippetLang,
  ingestEndpoint,
  SNIPPET_LANGS,
  type SnippetLang,
} from "./snippets";

// Эталоны — тексты сниппетов из 14-ux-ui-spec.md S-10 (платформа ios, ключ-пример, боевой URL).
// vitest запускается из web/; import.meta.url в jsdom — не file:, поэтому путь от cwd.
const SPEC = readFileSync(
  resolve(process.cwd(), "../docs/discovery/2026-10-13-appmetrica-online/14-ux-ui-spec.md"),
  "utf8",
);
const SPEC_KEY = "pk_7Hq2x9AbLm3Np4Qr5St6Uv7Wx8Yz9Ab0";
const SPEC_URL = "https://i.appmetrica.online/v1/events";

const SPEC_LABELS: Record<SnippetLang, string> = {
  curl: "curl (во всех сниппетах",
  swift: "Swift (iOS",
  kotlin: "Kotlin (Android",
  javascript: "JavaScript (браузер",
  go: "Go (боты",
};

// specSnippet — содержимое первого fenced-блока после строки-подписи сниппета.
function specSnippet(lang: SnippetLang): string {
  const start = SPEC.indexOf(`\n${SPEC_LABELS[lang]}`);
  expect(start, `подпись сниппета ${lang} в 14-ux-ui-spec.md`).toBeGreaterThan(0);
  const open = SPEC.indexOf("```", start);
  const body = SPEC.indexOf("\n", open) + 1;
  const close = SPEC.indexOf("\n```", body);
  return SPEC.slice(body, close + 1);
}

// curlJson — JSON-часть heredoc curl-сниппета с подставленными значениями shell.
function curlJson(snippet: string): unknown {
  const json = snippet.slice(snippet.indexOf("<<JSON\n") + 7, snippet.indexOf("\nJSON\n"));
  return JSON.parse(
    json
      .replaceAll("$NOW", "2026-10-13T09:00:00.000Z")
      .replaceAll("$(uuidgen)", "0199d6a1-7c2e-7b1a-9f00-3c1d2e4f5a61"),
  );
}

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("snippets", () => {
  it("renders all five snippets with key and url", () => {
    expect(SNIPPET_LANGS).toEqual(["curl", "swift", "kotlin", "javascript", "go"]);

    for (const lang of SNIPPET_LANGS) {
      const snippet = buildSnippet(lang, {
        ingestUrl: SPEC_URL,
        writeKey: SPEC_KEY,
        platform: "ios",
      });
      expect(snippet, lang).toBe(specSnippet(lang));
    }

    const key = "pk_AAAAbbbbCCCCddddEEEEffffGGGGhhhh";
    const url = "http://localhost:8081/v1/events";
    for (const lang of SNIPPET_LANGS) {
      const snippet = buildSnippet(lang, { ingestUrl: url, writeKey: key, platform: "ios" });
      expect(snippet, lang).toContain(key);
      expect(snippet, lang).toContain(url);
      expect(snippet, lang).not.toContain(SPEC_KEY);
      expect(snippet, lang).not.toContain(SPEC_URL);
      expect(snippet, lang).not.toMatch(/YOUR_|<WRITE_KEY>|\{\{\s*\w+\s*\}\}|pk_x{4}/);
    }

    const body = curlJson(buildSnippet("curl", { ingestUrl: url, writeKey: key, platform: "ios" }));
    expect(body).toMatchObject({
      context: { platform: "ios", environment: "development" },
      events: [{ name: "session_start" }],
    });
  });

  it("changes only context values of curl for other platforms", () => {
    const input = { ingestUrl: SPEC_URL, writeKey: SPEC_KEY } as const;
    const cases = [
      ["android", "Android", "phone"],
      ["macos", "macOS", "desktop"],
      ["web", "Web", "desktop"],
      ["windows", "Windows", "desktop"],
      ["linux", "Linux", "desktop"],
      ["bot", "Linux", "server"],
      ["server", "Linux", "server"],
      ["other", "Other", "unknown"],
    ] as const;
    const ios = buildSnippet("curl", { ...input, platform: "ios" });

    for (const [platform, osName, deviceType] of cases) {
      const snippet = buildSnippet("curl", { ...input, platform });
      expect(curlJson(snippet), platform).toMatchObject({
        context: { platform, os_name: osName, device_type: deviceType },
      });
      const normalized = snippet
        .replace(`"platform": "${platform}"`, `"platform": "ios"`)
        .replace(`"os_name": "${osName}"`, `"os_name": "iOS"`)
        .replace(`"device_type": "${deviceType}"`, `"device_type": "phone"`);
      expect(normalized, platform).toBe(ios);
    }
  });

  it("uses the app platform inside Swift and Go snippets", () => {
    const input = { ingestUrl: SPEC_URL, writeKey: SPEC_KEY } as const;
    expect(buildSnippet("swift", { ...input, platform: "macos" })).toContain('platform: "macos"');
    expect(buildSnippet("swift", { ...input, platform: "ios" })).toContain('platform: "ios"');
    expect(buildSnippet("go", { ...input, platform: "server" })).toContain(
      '"platform":    "server"',
    );
    expect(buildSnippet("go", { ...input, platform: "bot" })).toContain('"platform":    "bot"');
  });

  it("picks the default language by platform", () => {
    expect(defaultSnippetLang("ios")).toBe("swift");
    expect(defaultSnippetLang("macos")).toBe("swift");
    expect(defaultSnippetLang("android")).toBe("kotlin");
    expect(defaultSnippetLang("web")).toBe("javascript");
    expect(defaultSnippetLang("bot")).toBe("go");
    expect(defaultSnippetLang("server")).toBe("go");
    expect(defaultSnippetLang("windows")).toBe("curl");
    expect(defaultSnippetLang("other")).toBe("curl");
  });

  it("builds the ingest endpoint from NEXT_PUBLIC_INGEST_URL", () => {
    vi.stubEnv("NEXT_PUBLIC_INGEST_URL", "http://localhost:8081/");
    expect(ingestEndpoint()).toBe("http://localhost:8081/v1/events");
    vi.stubEnv("NEXT_PUBLIC_INGEST_URL", "");
    expect(ingestEndpoint()).toBe("https://i.appmetrica.online/v1/events");
  });
});
