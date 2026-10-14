import { describe, expect, it } from "vitest";
import {
  addDays,
  allowedGranularities,
  coerceGranularity,
  isIsoDate,
  presetPeriod,
  rangeDays,
  todayIn,
  yesterdayIn,
} from "./period";

describe("period", () => {
  it("adds days across month and year boundaries", () => {
    expect(addDays("2026-10-12", -7)).toBe("2026-10-05");
    expect(addDays("2026-10-31", 1)).toBe("2026-11-01");
    expect(addDays("2026-01-01", -1)).toBe("2025-12-31");
    expect(addDays("2028-02-28", 1)).toBe("2028-02-29");
  });

  it("computes today and yesterday in the given time zone", () => {
    const now = new Date("2026-10-12T22:30:00Z");
    expect(todayIn("Europe/Moscow", now)).toBe("2026-10-13");
    expect(yesterdayIn("Europe/Moscow", now)).toBe("2026-10-12");
    expect(todayIn("America/New_York", now)).toBe("2026-10-12");
    expect(yesterdayIn("UTC", now)).toBe("2026-10-11");
  });

  it("falls back to UTC for an unknown time zone", () => {
    expect(todayIn("Mars/Base", new Date("2026-10-12T22:30:00Z"))).toBe("2026-10-12");
  });

  it("accepts only real calendar dates", () => {
    expect(isIsoDate("2026-10-12")).toBe(true);
    expect(isIsoDate("2026-02-30")).toBe(false);
    expect(isIsoDate("2026-13-01")).toBe(false);
    expect(isIsoDate("12.10.2026")).toBe(false);
  });
});

describe("report periods", () => {
  const today = "2026-10-13";

  it("resolves presets in the app time zone, ending today inclusive", () => {
    expect(presetPeriod("today", today)).toEqual({ preset: "today", from: today, to: today });
    expect(presetPeriod("yesterday", today)).toEqual({
      preset: "yesterday",
      from: "2026-10-12",
      to: "2026-10-12",
    });
    expect(presetPeriod("7d", today)).toEqual({ preset: "7d", from: "2026-10-07", to: today });
    expect(presetPeriod("30d", today)).toEqual({ preset: "30d", from: "2026-09-14", to: today });
    expect(presetPeriod("90d", today)).toEqual({ preset: "90d", from: "2026-07-16", to: today });
  });

  it("counts range length in days inclusive", () => {
    expect(rangeDays("2026-10-13", "2026-10-13")).toBe(1);
    expect(rangeDays("2026-10-07", "2026-10-13")).toBe(7);
    expect(rangeDays("2025-10-13", "2026-10-13")).toBe(366);
    expect(rangeDays("2026-10-13", "2026-10-12")).toBe(0);
  });

  it("allows granularities per period", () => {
    const custom = (from: string, to: string) => ({ preset: "custom" as const, from, to });
    expect(allowedGranularities(presetPeriod("today", today))).toEqual(["hour"]);
    expect(allowedGranularities(presetPeriod("yesterday", today))).toEqual(["hour"]);
    expect(allowedGranularities(presetPeriod("7d", today))).toEqual(["hour", "day"]);
    expect(allowedGranularities(presetPeriod("30d", today))).toEqual(["day", "week"]);
    expect(allowedGranularities(presetPeriod("90d", today))).toEqual(["day", "week", "month"]);
    expect(allowedGranularities(custom("2026-10-07", "2026-10-13"))).toEqual([
      "hour",
      "day",
      "week",
    ]);
    expect(allowedGranularities(custom("2026-10-06", "2026-10-13"))).toEqual(["day", "week"]);
    expect(allowedGranularities(custom("2026-08-15", "2026-10-13"))).toEqual([
      "day",
      "week",
      "month",
    ]);
  });

  it("replaces a granularity with the nearest allowed one", () => {
    expect(coerceGranularity("day", ["hour"])).toBe("hour");
    expect(coerceGranularity("hour", ["day", "week"])).toBe("day");
    expect(coerceGranularity("month", ["day", "week"])).toBe("week");
    expect(coerceGranularity("week", ["hour", "day"])).toBe("day");
    expect(coerceGranularity("week", ["day", "week", "month"])).toBe("week");
  });
});
