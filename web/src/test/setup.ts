import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, beforeEach, vi } from "vitest";
import { server } from "./msw";

// jsdom не реализует API, которые нужны Radix Popover и cmdk (AppSwitcher, TimezoneSelect, FilterBar).
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver ??= ResizeObserverStub;
if (typeof Element !== "undefined") {
  Element.prototype.scrollIntoView ??= () => {};
  // Radix Select (S-04 «Платформа») открывается по pointer-событиям и проверяет захват указателя.
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.releasePointerCapture ??= () => {};
}

// Запрос без хендлера — ошибка теста: сеть в unit-тестах не используется (22-test-strategy.md §7).
beforeAll(() => server.listen({ onUnhandledFrame: "error" }));
// `make test` экспортирует .env с локальными NEXT_PUBLIC_*_URL; тесты ждут production-адреса,
// пока сами не подставят свой через vi.stubEnv.
beforeEach(() => {
  vi.stubEnv("NEXT_PUBLIC_API_URL", "");
  vi.stubEnv("NEXT_PUBLIC_INGEST_URL", "");
});
afterEach(() => {
  server.resetHandlers();
  cleanup();
  vi.unstubAllEnvs();
});
afterAll(() => server.close());
