import { act, renderHook } from "@testing-library/react";
import { type OnUrlUpdateFunction, withNuqsTestingAdapter } from "nuqs/adapters/testing";
import { describe, expect, it, vi } from "vitest";
import {
  type AudienceState,
  loadAudienceSearch,
  periodSearch,
  reportParams,
  resolveAudienceState,
  serializeAudienceSearch,
  useAudienceState,
} from "./url-state";

const TODAY = "2026-10-13";

const resolve = (search: string) => resolveAudienceState(loadAudienceSearch(search), TODAY);

describe("url-state", () => {
  it("round-trips filters through the URL", async () => {
    const url = "?from=2026-10-01&to=2026-10-12&platform=ios&platform=android";

    const search = loadAudienceSearch(url);
    const state = resolveAudienceState(search, TODAY);
    expect(state.period).toEqual({ preset: "custom", from: "2026-10-01", to: "2026-10-12" });
    expect(state.filters).toEqual({ platform: ["ios", "android"] });
    expect(serializeAudienceSearch(search)).toBe(url);

    // «Перезагрузка» — тот же URL снова читается хуком: состояние совпадает.
    const onUrlUpdate = vi.fn<OnUrlUpdateFunction>();
    const { result } = renderHook(() => useAudienceState("Europe/Moscow", TODAY), {
      wrapper: withNuqsTestingAdapter({ searchParams: url, hasMemory: true, onUrlUpdate }),
    });
    expect(result.current.state).toEqual(state);

    await act(() => result.current.update({ app_version: ["2.1.0", "2.1.1"] }));

    expect(onUrlUpdate).toHaveBeenCalledOnce();
    const update = onUrlUpdate.mock.calls[0]?.[0];
    expect(update?.queryString).toBe(`${url}&app_version=2.1.0&app_version=2.1.1`);
    // Каждое изменение — новая запись истории: «Назад» возвращает прежнее состояние.
    expect(update?.options.history).toBe("push");
    expect(result.current.state.filters).toEqual({
      platform: ["ios", "android"],
      app_version: ["2.1.0", "2.1.1"],
    });
  });

  it("uses defaults for an empty URL", () => {
    const state: AudienceState = resolve("");
    expect(state).toEqual({
      period: { preset: "7d", from: "2026-10-07", to: TODAY },
      granularity: "day",
      granularityAdjusted: false,
      compare: false,
      metric: "active",
      dim: "app_version",
      bmetric: "active",
      filters: {},
      env: "production",
      unit: "persons",
    });
    expect(serializeAudienceSearch(loadAudienceSearch(""))).toBe("");
  });

  it("reads every report parameter", () => {
    const state = resolve(
      "?period=90d&g=month&compare=1&metric=sessions&dim=country&bmetric=new&os_name=iOS&country=RU&country=KZ&env=development&unit=installs",
    );
    expect(state).toMatchObject({
      period: { preset: "90d", from: "2026-07-16", to: TODAY },
      granularity: "month",
      granularityAdjusted: false,
      compare: true,
      metric: "sessions",
      dim: "country",
      bmetric: "new",
      filters: { os_name: ["iOS"], country: ["RU", "KZ"] },
      env: "development",
      unit: "installs",
    });
  });

  it("ignores invalid values", () => {
    const state = resolve(
      "?period=year&g=decade&metric=revenue&dim=ip&bmetric=avg_duration_ms&platform=symbian&platform=ios&country=ru&env=staging&unit=devices",
    );
    expect(state.period).toEqual({ preset: "7d", from: "2026-10-07", to: TODAY });
    expect(state.granularity).toBe("day");
    expect(state.metric).toBe("active");
    expect(state.dim).toBe("app_version");
    expect(state.bmetric).toBe("active");
    expect(state.filters).toEqual({ platform: ["ios"] });
    expect(state.env).toBe("production");
    expect(state.unit).toBe("persons");
  });

  it("falls back to 7 days for an incomplete or reversed custom range", () => {
    const week = { preset: "7d", from: "2026-10-07", to: TODAY };
    expect(resolve("?period=custom").period).toEqual(week);
    expect(resolve("?from=2026-10-01").period).toEqual(week);
    expect(resolve("?from=2026-02-30&to=2026-10-12").period).toEqual(week);
    expect(resolve("?from=2026-10-12&to=2026-10-01").period).toEqual(week);
    // Пресет важнее дат: ?period=30d&from=… — это 30 дней.
    expect(resolve("?period=30d&from=2026-10-01&to=2026-10-02").period.preset).toBe("30d");
  });

  it("keeps an oversized custom range so the API can report range_too_large", () => {
    expect(resolve("?from=2024-01-01&to=2026-10-12").period).toEqual({
      preset: "custom",
      from: "2024-01-01",
      to: "2026-10-12",
    });
  });

  it("replaces a granularity the period does not allow", () => {
    expect(resolve("?period=today")).toMatchObject({
      granularity: "hour",
      granularityAdjusted: true,
    });
    expect(resolve("?period=30d&g=hour")).toMatchObject({
      granularity: "day",
      granularityAdjusted: true,
    });
    expect(resolve("?period=7d&g=hour")).toMatchObject({
      granularity: "hour",
      granularityAdjusted: false,
    });
  });

  it("maps a period to URL parameters", () => {
    expect(periodSearch({ preset: "30d", from: "2026-09-14", to: TODAY })).toEqual({
      period: "30d",
      from: null,
      to: null,
    });
    expect(periodSearch({ preset: "custom", from: "2026-10-01", to: "2026-10-05" })).toEqual({
      period: "custom",
      from: "2026-10-01",
      to: "2026-10-05",
    });
  });

  it("builds API parameters without empty filters", () => {
    expect(reportParams(resolve("?period=yesterday&platform=ios&unit=installs"))).toEqual({
      from: "2026-10-12",
      to: "2026-10-12",
      environment: "production",
      unit: "installs",
      platform: ["ios"],
    });
    expect(reportParams(resolve(""))).toEqual({
      from: "2026-10-07",
      to: TODAY,
      environment: "production",
      unit: "persons",
    });
  });

  it("computes today in the app time zone when not given", () => {
    vi.useFakeTimers({ now: new Date("2026-10-12T22:30:00Z"), shouldAdvanceTime: true });
    try {
      const { result } = renderHook(() => useAudienceState("Europe/Moscow"), {
        wrapper: withNuqsTestingAdapter({ searchParams: "?period=today" }),
      });
      expect(result.current.state.period).toEqual({
        preset: "today",
        from: TODAY,
        to: TODAY,
      });
    } finally {
      vi.useRealTimers();
    }
  });
});
