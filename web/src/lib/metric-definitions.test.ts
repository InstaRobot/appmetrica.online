import { describe, expect, it } from "vitest";
import { METRIC_DEFINITIONS } from "./metric-definitions";

describe("metric definitions", () => {
  it("names metrics as in the UI glossary", () => {
    expect(METRIC_DEFINITIONS.active.label).toBe("Активные");
    expect(METRIC_DEFINITIONS.new.label).toBe("Новые");
    expect(METRIC_DEFINITIONS.sessions.label).toBe("Сессии");
    expect(METRIC_DEFINITIONS.avg_duration_ms.label).toBe("Ср. длительность");
    expect(METRIC_DEFINITIONS.median_duration_ms.label).toBe("Медиана");
    expect(METRIC_DEFINITIONS.active_now.label).toBe("Активны сейчас");
  });

  it("has a tooltip definition for every metric", () => {
    for (const [key, { label, definition }] of Object.entries(METRIC_DEFINITIONS)) {
      expect(label, key).not.toBe("");
      expect(definition.length, key).toBeGreaterThan(20);
      expect(definition, key).toMatch(/\.$/);
    }
    expect(METRIC_DEFINITIONS.retention_d1.definition).toMatch(/ровно/);
    expect(METRIC_DEFINITIONS.active_now.definition).toMatch(/30 минут/);
  });
});
