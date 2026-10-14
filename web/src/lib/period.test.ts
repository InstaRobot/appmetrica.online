import { describe, expect, it } from "vitest";
import { addDays, isIsoDate, todayIn, yesterdayIn } from "./period";

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
