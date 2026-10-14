import type { Platform } from "@/lib/api/gen/model";

// Сниппеты онбординга S-10 (FR-017, 14-ux-ui-spec.md «Сниппеты»). Тексты для платформы ios совпадают
// с эталоном спецификации (snippets.test.ts); для других платформ в curl меняются только значения
// context, в Swift и Go — значение platform.

export const SNIPPET_LANGS = ["curl", "swift", "kotlin", "javascript", "go"] as const;
export type SnippetLang = (typeof SNIPPET_LANGS)[number];

export const SNIPPET_LABELS: Record<SnippetLang, string> = {
  curl: "curl",
  swift: "Swift",
  kotlin: "Kotlin",
  javascript: "JavaScript",
  go: "Go",
};

// Рецепты документации с офлайн-очередью и сессиями (07-project-structure.md, content/docs/recipes).
export const SNIPPET_RECIPES: Record<SnippetLang, string> = {
  curl: "/docs/recipes/curl",
  swift: "/docs/recipes/ios-swift",
  kotlin: "/docs/recipes/android-kotlin",
  javascript: "/docs/recipes/web-js",
  go: "/docs/recipes/go",
};

export type SnippetInput = { ingestUrl: string; writeKey: string; platform: Platform };

const DEFAULT_INGEST_URL = "https://i.appmetrica.online";

// ingestEndpoint — `NEXT_PUBLIC_INGEST_URL` + `/v1/events`; без переменной — боевой адрес.
export function ingestEndpoint(): string {
  const base = process.env.NEXT_PUBLIC_INGEST_URL || DEFAULT_INGEST_URL;
  return `${base.replace(/\/+$/, "")}/v1/events`;
}

// ios/macos → Swift, android → Kotlin, web → JavaScript, bot/server → Go, иначе curl.
export function defaultSnippetLang(platform: Platform): SnippetLang {
  switch (platform) {
    case "ios":
    case "macos":
      return "swift";
    case "android":
      return "kotlin";
    case "web":
      return "javascript";
    case "bot":
    case "server":
      return "go";
    default:
      return "curl";
  }
}

type CurlContext = { platform: Platform; osName: string; deviceType: string };

const CURL_CONTEXT: Record<Platform, Omit<CurlContext, "platform">> = {
  ios: { osName: "iOS", deviceType: "phone" },
  android: { osName: "Android", deviceType: "phone" },
  macos: { osName: "macOS", deviceType: "desktop" },
  web: { osName: "Web", deviceType: "desktop" },
  windows: { osName: "Windows", deviceType: "desktop" },
  linux: { osName: "Linux", deviceType: "desktop" },
  bot: { osName: "Linux", deviceType: "server" },
  server: { osName: "Linux", deviceType: "server" },
  other: { osName: "Other", deviceType: "unknown" },
};

const swiftPlatform = (platform: Platform) => (platform === "macos" ? "macos" : "ios");
const goPlatform = (platform: Platform) => (platform === "server" ? "server" : "bot");

function curlSnippet({ ingestUrl, writeKey, platform }: SnippetInput): string {
  const ctx: CurlContext = { platform, ...CURL_CONTEXT[platform] };
  return `NOW=$(date -u +"%Y-%m-%dT%H:%M:%S.000Z")
curl -sS -X POST "${ingestUrl}" \\
  -H "Content-Type: application/json" \\
  -H "X-App-Key: ${writeKey}" \\
  --data-binary @- <<JSON
{
  "sent_at": "$NOW",
  "context": {
    "platform": "${ctx.platform}",
    "os_name": "${ctx.osName}",
    "os_version": "26.0",
    "app_version": "1.0.0",
    "device_type": "${ctx.deviceType}",
    "locale": "ru-RU",
    "environment": "development"
  },
  "events": [
    {
      "insert_id": "$(uuidgen)",
      "name": "session_start",
      "timestamp": "$NOW",
      "install_id": "$(uuidgen)",
      "session_id": "$(uuidgen)"
    }
  ]
}
JSON
# Ожидаемый ответ: {"accepted":1,"rejected":[],"warnings":[],"server_time":"…"}
`;
}

function swiftSnippet({ ingestUrl, writeKey, platform }: SnippetInput): string {
  return `import Foundation
import UIKit

struct EventBatch: Encodable, Sendable { let sentAt: Date; let context: EventContext; let events: [Event] }
struct EventContext: Encodable, Sendable {
    let platform, osName, osVersion, appVersion, appBuild, deviceModel, deviceType, locale, environment: String
}
struct Event: Encodable, Sendable {
    let insertId: UUID; let name: String; let timestamp: Date
    let installId: UUID; let sessionId: UUID; let userId: String?; let durationMs: Int?
}

enum Analytics {
    static let endpoint = URL(string: "${ingestUrl}")!
    static let writeKey = "${writeKey}"

    static func send(_ events: [Event], context: EventContext) async throws {
        let encoder = JSONEncoder()
        encoder.keyEncodingStrategy = .convertToSnakeCase
        encoder.dateEncodingStrategy = .iso8601
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue(writeKey, forHTTPHeaderField: "X-App-Key")
        request.httpBody = try encoder.encode(EventBatch(sentAt: Date(), context: context, events: events))
        let (_, response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
            throw URLError(.badServerResponse)
        }
    }

    @MainActor static func currentContext() -> EventContext {
        let info = Bundle.main.infoDictionary ?? [:]
        #if DEBUG
        let environment = "development"
        #else
        let environment = "production"
        #endif
        return EventContext(
            platform: "${swiftPlatform(platform)}",
            osName: UIDevice.current.systemName,
            osVersion: UIDevice.current.systemVersion,
            appVersion: info["CFBundleShortVersionString"] as? String ?? "",
            appBuild: info["CFBundleVersion"] as? String ?? "",
            deviceModel: hardwareModel(),
            deviceType: UIDevice.current.userInterfaceIdiom == .pad ? "tablet" : "phone",
            locale: Locale.current.identifier.replacingOccurrences(of: "_", with: "-"),
            environment: environment
        )
    }

    static func hardwareModel() -> String {
        var systemInfo = utsname()
        uname(&systemInfo)
        return withUnsafeBytes(of: &systemInfo.machine) { raw in
            String(decoding: raw.prefix(while: { $0 != 0 }), as: UTF8.self)
        }
    }
}

// Старт сессии при выходе на передний план. InstallID.current — UUID из Keychain
// (рецепт хранения и офлайн-очередь: /docs/recipes/ios-swift).
// let context = await Analytics.currentContext()
// try await Analytics.send([Event(insertId: UUID(), name: "session_start", timestamp: Date(),
//     installId: InstallID.current, sessionId: UUID(), userId: nil, durationMs: nil)], context: context)
`;
}

function kotlinSnippet({ ingestUrl, writeKey }: SnippetInput): string {
  return `import android.content.Context
import android.os.Build
import org.json.JSONArray
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL
import java.time.Instant
import java.util.Locale
import java.util.UUID

object Analytics {
    private const val ENDPOINT = "${ingestUrl}"
    private const val WRITE_KEY = "${writeKey}"

    // Вызывать из фонового потока (Dispatchers.IO). Возвращает HTTP-код ответа.
    fun send(events: JSONArray, context: JSONObject): Int {
        val body = JSONObject()
            .put("sent_at", Instant.now().toString())
            .put("context", context)
            .put("events", events)
        val conn = URL(ENDPOINT).openConnection() as HttpURLConnection
        return try {
            conn.requestMethod = "POST"
            conn.doOutput = true
            conn.connectTimeout = 10_000
            conn.readTimeout = 10_000
            conn.setRequestProperty("Content-Type", "application/json")
            conn.setRequestProperty("X-App-Key", WRITE_KEY)
            conn.outputStream.use { it.write(body.toString().toByteArray(Charsets.UTF_8)) }
            conn.responseCode
        } finally {
            conn.disconnect()
        }
    }

    fun context(app: Context, versionName: String, versionCode: Long, debug: Boolean): JSONObject =
        JSONObject()
            .put("platform", "android")
            .put("os_name", "Android")
            .put("os_version", Build.VERSION.RELEASE)
            .put("app_version", versionName)
            .put("app_build", versionCode.toString())
            .put("device_model", "\${Build.MANUFACTURER} \${Build.MODEL}")
            .put("device_type", if (app.resources.configuration.smallestScreenWidthDp >= 600) "tablet" else "phone")
            .put("locale", Locale.getDefault().toLanguageTag())
            .put("environment", if (debug) "development" else "production")

    fun sessionStart(installId: UUID, sessionId: UUID, userId: String?): JSONObject =
        JSONObject()
            .put("insert_id", UUID.randomUUID().toString())
            .put("name", "session_start")
            .put("timestamp", Instant.now().toString())
            .put("install_id", installId.toString())
            .put("session_id", sessionId.toString())
            .apply { if (userId != null) put("user_id", userId) }
}
`;
}

function javascriptSnippet({ ingestUrl, writeKey }: SnippetInput): string {
  return `const ENDPOINT = "${ingestUrl}";
const WRITE_KEY = "${writeKey}";

function installId() {
  let id = localStorage.getItem("am_install_id");
  if (!id) {
    id = crypto.randomUUID();
    localStorage.setItem("am_install_id", id);
  }
  return id;
}

const context = {
  platform: "web",
  app_version: "1.0.0",
  device_type: "desktop",
  locale: navigator.language,
  environment: location.hostname === "localhost" ? "development" : "production",
};

let sessionId = crypto.randomUUID();
let sessionStartedAt = Date.now();

function batch(events) {
  return JSON.stringify({ app_key: WRITE_KEY, sent_at: new Date().toISOString(), context, events });
}

function sessionStart() {
  sessionId = crypto.randomUUID();
  sessionStartedAt = Date.now();
  const event = {
    insert_id: crypto.randomUUID(),
    name: "session_start",
    timestamp: new Date().toISOString(),
    install_id: installId(),
    session_id: sessionId,
  };
  fetch(ENDPOINT, { method: "POST", headers: { "Content-Type": "text/plain" }, body: batch([event]), keepalive: true });
}

function sessionEnd() {
  const event = {
    insert_id: crypto.randomUUID(),
    name: "session_end",
    timestamp: new Date().toISOString(),
    install_id: installId(),
    session_id: sessionId,
    duration_ms: Date.now() - sessionStartedAt,
  };
  navigator.sendBeacon(ENDPOINT, new Blob([batch([event])], { type: "text/plain" }));
}

sessionStart();
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "hidden") sessionEnd();
  else sessionStart();
});
`;
}

function goSnippet({ ingestUrl, writeKey, platform }: SnippetInput): string {
  return `package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	endpoint = "${ingestUrl}"
	writeKey = "${writeKey}"
)

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func main() {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	batch := map[string]any{
		"sent_at": now,
		"context": map[string]any{
			"platform":    "${goPlatform(platform)}",
			"app_version": "1.0.0",
			"device_type": "server",
			"environment": "development",
			"ip":          "none", // для s2s: передайте IP пользователя или "none"
		},
		"events": []map[string]any{{
			"insert_id":  newUUID(),
			"name":       "session_start",
			"timestamp":  now,
			"install_id": newUUID(), // для бота — стабильный UUID на пользователя чата
			"session_id": newUUID(),
		}},
	}
	body, err := json.Marshal(batch)
	if err != nil {
		log.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-App-Key", writeKey)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	log.Println("status:", resp.Status)
}
`;
}

const BUILDERS: Record<SnippetLang, (input: SnippetInput) => string> = {
  curl: curlSnippet,
  swift: swiftSnippet,
  kotlin: kotlinSnippet,
  javascript: javascriptSnippet,
  go: goSnippet,
};

export function buildSnippet(lang: SnippetLang, input: SnippetInput): string {
  return BUILDERS[lang](input);
}
