import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { WriteKey } from "@/lib/api/gen/model";
import { renderWithLocale } from "@/test/rtl";
import { activeWriteKeyFor, PlatformIntegrationSection } from "./platform-integration-section";

const APP_ID = "0192aaaa-0000-7000-8000-000000000012";
const INGEST_URL = "http://localhost:8081/v1/events";

function writeKey(overrides: Partial<WriteKey> = {}): WriteKey {
  return {
    id: "0192cccc-0000-7000-8000-000000000001",
    app_id: APP_ID,
    key: "pk_AAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaa",
    label: "",
    platform: null,
    created_at: "2026-10-01T00:00:00Z",
    revoked_at: null,
    last_used_at: null,
    ...overrides,
  };
}

describe("activeWriteKeyFor", () => {
  const shared = writeKey({ id: "shared", created_at: "2026-09-01T00:00:00Z" });
  const iosOld = writeKey({
    id: "ios-old",
    platform: "ios",
    created_at: "2026-09-02T00:00:00Z",
    revoked_at: "2026-09-03T00:00:00Z",
  });
  const iosNew = writeKey({ id: "ios-new", platform: "ios", created_at: "2026-10-05T00:00:00Z" });
  const iosNewer = writeKey({ id: "ios-2", platform: "ios", created_at: "2026-10-06T00:00:00Z" });
  const android = writeKey({ id: "android", platform: "android" });

  it("prefers the oldest active key of the platform", () => {
    expect(activeWriteKeyFor([iosNewer, shared, iosOld, iosNew], "ios")?.id).toBe("ios-new");
  });

  it("falls back to the oldest active key without a platform", () => {
    const sharedNewer = writeKey({ id: "shared-2", created_at: "2026-10-09T00:00:00Z" });
    expect(activeWriteKeyFor([sharedNewer, android, iosNew, shared], "web")?.id).toBe("shared");
  });

  it("returns nothing when only other platforms or revoked keys are left", () => {
    expect(activeWriteKeyFor([android, iosNew], "web")).toBeUndefined();
    expect(
      activeWriteKeyFor([writeKey({ revoked_at: "2026-10-02T00:00:00Z" }), iosOld], "ios"),
    ).toBeUndefined();
    expect(activeWriteKeyFor([], "ios")).toBeUndefined();
  });
});

describe("PlatformIntegrationSection", () => {
  it("shows the key, snippets of the platform and the waiting status", () => {
    render(
      <PlatformIntegrationSection
        appId={APP_ID}
        platform="android"
        writeKey={writeKey({ platform: "android" })}
        received={false}
        ingestUrl={INGEST_URL}
      />,
    );

    const section = screen.getByRole("region", { name: "Android" });
    expect(within(section).getByRole("textbox", { name: "write-ключ" })).toHaveValue(
      "pk_AAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaa",
    );
    expect(within(section).getByRole("tab", { name: "Kotlin" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(within(section).getByLabelText("Пример кода на Kotlin").textContent).toContain(
      '.put("platform", "android")',
    );
    expect(within(section).getByRole("status", { name: "Статус Android" })).toHaveTextContent(
      "Ждём первое событие с Android",
    );
  });

  it("marks the platform as received", () => {
    render(
      <PlatformIntegrationSection
        appId={APP_ID}
        platform="web"
        writeKey={writeKey()}
        received
        ingestUrl={INGEST_URL}
      />,
    );

    expect(screen.getByRole("status", { name: "Статус Веб" })).toHaveTextContent(
      "Получено первое событие",
    );
  });

  it("warns without an active key and hides the snippets", () => {
    render(
      <PlatformIntegrationSection
        appId={APP_ID}
        platform="android"
        writeKey={undefined}
        received={false}
        ingestUrl={INGEST_URL}
      />,
    );

    const section = screen.getByRole("region", { name: "Android" });
    const alert = within(section).getByRole("alert");
    expect(alert).toHaveTextContent("Для Android нет активного ключа");
    expect(within(alert).getByRole("link", { name: "Создать ключ для Android" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/settings#write-keys`,
    );
    expect(within(section).queryByRole("tablist")).not.toBeInTheDocument();
    expect(within(section).queryByRole("textbox", { name: "write-ключ" })).not.toBeInTheDocument();
  });
});

describe("PlatformIntegrationSection in English", () => {
  it("shows the key, the snippets and the waiting status", () => {
    renderWithLocale(
      <PlatformIntegrationSection
        appId={APP_ID}
        platform="android"
        writeKey={writeKey({ platform: "android" })}
        received={false}
        ingestUrl={INGEST_URL}
      />,
      "en",
    );

    const section = screen.getByRole("region", { name: "Android" });
    expect(
      within(section).getByText("Write key (public, embedded in the app)"),
    ).toBeInTheDocument();
    expect(within(section).getByRole("textbox", { name: "write key" })).toHaveValue(
      "pk_AAAAaaaaAAAAaaaaAAAAaaaaAAAAaaaa",
    );
    expect(within(section).getByLabelText("Code example in Kotlin").textContent).toContain(
      "// Call from a background thread (Dispatchers.IO).",
    );
    expect(within(section).getByRole("status", { name: "Android status" })).toHaveTextContent(
      "Waiting for the first event from Android",
    );
  });

  it("marks the platform as received", () => {
    renderWithLocale(
      <PlatformIntegrationSection
        appId={APP_ID}
        platform="ios"
        writeKey={writeKey()}
        received
        ingestUrl={INGEST_URL}
      />,
      "en",
    );

    expect(screen.getByRole("status", { name: "iOS status" })).toHaveTextContent(
      "First event received",
    );
  });

  it("warns without an active key", () => {
    renderWithLocale(
      <PlatformIntegrationSection
        appId={APP_ID}
        platform="android"
        writeKey={undefined}
        received={false}
        ingestUrl={INGEST_URL}
      />,
      "en",
    );

    const alert = within(screen.getByRole("region", { name: "Android" })).getByRole("alert");
    expect(alert).toHaveTextContent("No active key for Android.");
    expect(within(alert).getByRole("link", { name: "Create a key for Android" })).toHaveAttribute(
      "href",
      `/apps/${APP_ID}/settings#write-keys`,
    );
  });
});
