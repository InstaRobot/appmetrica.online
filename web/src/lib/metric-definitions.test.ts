import { renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { LOCALES } from "@/lib/i18n/locale";
import { getMessages } from "@/messages";
import { useMetricDefinitions } from "./metric-definitions";

describe("metric definitions", () => {
  it("names metrics as in the UI glossary", () => {
    const { result } = renderHook(() => useMetricDefinitions());
    expect(result.current.active.label).toBe("Активные");
    expect(result.current.new.label).toBe("Новые");
    expect(result.current.sessions.label).toBe("Сессии");
    expect(result.current.avg_duration_ms.label).toBe("Ср. длительность");
    expect(result.current.median_duration_ms.label).toBe("Медиана");
    expect(result.current.active_now.label).toBe("Активны сейчас");
  });

  it("names metrics in English", () => {
    const metrics = getMessages("en").metrics;
    expect(metrics.active.label).toBe("Active users");
    expect(metrics.new.label).toBe("New users");
    expect(metrics.sessions.label).toBe("Sessions");
    expect(metrics.avg_duration_ms.label).toBe("Avg. duration");
    expect(metrics.median_duration_ms.label).toBe("Median");
    expect(metrics.active_now.label).toBe("Active now");
  });

  it("has a tooltip definition for every metric in every language", () => {
    for (const locale of LOCALES) {
      for (const [key, { label, definition }] of Object.entries(getMessages(locale).metrics)) {
        expect(label, `${locale}.${key}`).not.toBe("");
        expect(definition.length, `${locale}.${key}`).toBeGreaterThan(20);
        expect(definition, `${locale}.${key}`).toMatch(/\.$/);
      }
    }
    expect(getMessages("ru").metrics.retention_d1.definition).toMatch(/ровно/);
    expect(getMessages("ru").metrics.active_now.definition).toMatch(/30 минут/);
    expect(getMessages("en").metrics.retention_d1.definition).toMatch(/exactly/);
    expect(getMessages("en").metrics.active_now.definition).toMatch(/30 minutes/);
  });
});
