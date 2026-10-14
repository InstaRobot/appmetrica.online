import { describe, expect, it } from "vitest";
import {
  formatBucket,
  formatClock,
  formatDate,
  formatDateRange,
  formatDateShort,
  formatDateTime,
  formatDelta,
  formatDuration,
  formatInt,
  formatPercent,
  formatRejectionRate,
  formatWeekdayDate,
  formatWeekRange,
  shortId,
} from "./format";

const NBSP = " ";

describe("format", () => {
  it("formats numbers, percents and durations in ru-RU", () => {
    expect(formatInt(12480)).toBe(`12${NBSP}480`);
    expect(formatPercent(0.381)).toBe(`38,1${NBSP}%`);
    expect(formatDuration(252000)).toBe(`4${NBSP}мин${NBSP}12${NBSP}с`);
  });

  it("formats integers", () => {
    expect(formatInt(0)).toBe("0");
    expect(formatInt(999)).toBe("999");
    expect(formatInt(1234567)).toBe(`1${NBSP}234${NBSP}567`);
    expect(formatInt(12480.6)).toBe(`12${NBSP}481`);
    expect(formatInt(-1500)).toBe(`-1${NBSP}500`);
  });

  it("formats percents with at most one fraction digit", () => {
    expect(formatPercent(0)).toBe(`0${NBSP}%`);
    expect(formatPercent(1)).toBe(`100${NBSP}%`);
    expect(formatPercent(0.14)).toBe(`14${NBSP}%`);
    expect(formatPercent(0.12345)).toBe(`12,3${NBSP}%`);
  });

  it.each([
    [0, `0${NBSP}с`],
    [999, `1${NBSP}с`],
    [45_000, `45${NBSP}с`],
    [59_600, `1${NBSP}мин`],
    [60_000, `1${NBSP}мин`],
    [3_600_000, `1${NBSP}ч`],
    [3_725_000, `1${NBSP}ч${NBSP}2${NBSP}мин`],
    [90_061_000, `25${NBSP}ч${NBSP}1${NBSP}мин`],
  ])("formats duration %d ms", (ms, want) => {
    expect(formatDuration(ms)).toBe(want);
  });

  it.each([
    ["null", null],
    ["undefined", undefined],
    ["NaN", Number.NaN],
    ["Infinity", Number.POSITIVE_INFINITY],
  ])("renders a dash for %s", (_name, value) => {
    expect(formatInt(value)).toBe("—");
    expect(formatPercent(value)).toBe("—");
    expect(formatDuration(value)).toBe("—");
  });

  it("renders a dash for negative durations", () => {
    expect(formatDuration(-1)).toBe("—");
  });

  it.each([
    [4.1, `+4,1${NBSP}%`],
    [-1.2, `\u22121,2${NBSP}%`],
    [12, `+12${NBSP}%`],
    [0, `0${NBSP}%`],
    [-0.04, `0${NBSP}%`],
    [null, "—"],
  ])("formats delta %s with an explicit sign", (value, want) => {
    expect(formatDelta(value)).toBe(want);
  });

  it("formats report dates with short russian months", () => {
    expect(formatDate("2026-10-12")).toBe(`12${NBSP}окт${NBSP}2026`);
    expect(formatDate("2026-05-01")).toBe(`1${NBSP}мая${NBSP}2026`);
    expect(formatDateShort("2026-09-29")).toBe(`29${NBSP}сен`);
    expect(formatWeekdayDate("2026-10-05")).toBe(`пн${NBSP}5${NBSP}окт`);
    expect(formatWeekdayDate("2026-10-11")).toBe(`вс${NBSP}11${NBSP}окт`);
  });

  it("formats timestamps as dates in the given time zone", () => {
    expect(formatDate("2026-09-20T22:30:00Z", "Europe/Moscow")).toBe(`21${NBSP}сен${NBSP}2026`);
    expect(formatDate("2026-09-20T22:30:00Z", "UTC")).toBe(`20${NBSP}сен${NBSP}2026`);
    expect(formatDate("garbage")).toBe("—");
  });
});

describe("report ranges and buckets", () => {
  it("labels ISO week cohorts by their days without a year", () => {
    expect(formatWeekRange("2026-09-08")).toBe(`8–14${NBSP}сен`);
    expect(formatWeekRange("2026-09-28")).toBe(`28${NBSP}сен – 4${NBSP}окт`);
    expect(formatWeekRange("2025-12-29")).toBe(`29${NBSP}дек – 4${NBSP}янв`);
    expect(formatWeekRange("bad")).toBe("—");
  });

  it("formats date ranges compactly", () => {
    expect(formatDateRange("2026-10-06", "2026-10-12")).toBe("6–12 окт 2026");
    expect(formatDateRange("2026-09-28", "2026-10-05")).toBe("28 сен – 5 окт 2026");
    expect(formatDateRange("2025-12-28", "2026-01-03")).toBe("28 дек 2025 – 3 янв 2026");
    expect(formatDateRange("2026-10-12", "2026-10-12")).toBe("12 окт 2026");
    expect(formatDateRange("bad", "2026-10-12")).toBe("—");
  });

  it("labels series buckets in the app time zone", () => {
    const ts = "2026-10-12T14:00:00+03:00";
    expect(formatBucket(ts, "hour", "Europe/Moscow")).toBe("12 окт, 14:00");
    expect(formatBucket(ts, "hour", "UTC")).toBe("12 окт, 11:00");
    expect(formatBucket(ts, "day", "Europe/Moscow")).toBe("12 окт");
    expect(formatBucket(ts, "week", "Europe/Moscow")).toBe("12 окт");
    expect(formatBucket("2026-01-01T00:00:00+03:00", "month", "Europe/Moscow")).toBe("янв 2026");
    expect(formatBucket("2026-10-12T14:00:00+03:00", "hour", "Mars/Base")).toBe("12 окт, 11:00");
    expect(formatBucket("nope", "day", "UTC")).toBe("—");
  });
});

describe("realtime formatting", () => {
  it("formats clock time with seconds in the app time zone", () => {
    expect(formatClock("2026-10-13T11:32:05Z", "Europe/Moscow")).toBe("14:32:05");
    expect(formatClock("2026-10-13T00:00:09Z", "UTC")).toBe("00:00:09");
    expect(formatClock("2026-10-13T11:32:05Z", "Nope/Zone")).toBe("11:32:05");
    expect(formatClock("nope", "UTC")).toBe("—");
  });

  it("shortens long ids to the first and last four characters", () => {
    expect(shortId("5d8e0000-0000-4000-8000-00000a3f4e5f")).toBe("5d8e…4e5f");
    expect(shortId("u:42")).toBe("u:42");
    expect(shortId("123456789")).toBe("123456789");
    expect(shortId("1234567890")).toBe("1234…7890");
    expect(shortId("")).toBe("—");
  });
});

describe("integration formatting", () => {
  it("formats rejection rate with two significant digits", () => {
    expect(formatRejectionRate(1204, 412905)).toBe(`0,29${NBSP}%`);
    expect(formatRejectionRate(12, 300112)).toBe(`0,004${NBSP}%`);
    expect(formatRejectionRate(1190, 98400)).toBe(`1,2${NBSP}%`);
    expect(formatRejectionRate(0, 10)).toBe(`0${NBSP}%`);
    expect(formatRejectionRate(5, 0)).toBe(`100${NBSP}%`);
    expect(formatRejectionRate(0, 0)).toBe("—");
  });
});

describe("profile formatting", () => {
  it("formats date and minute in the app timezone", () => {
    expect(formatDateTime("2026-10-12T11:01:30Z", "Europe/Moscow")).toBe(
      `12${NBSP}окт${NBSP}2026 14:01`,
    );
    expect(formatDateTime("2026-10-12T22:30:00Z", "Europe/Moscow")).toBe(
      `13${NBSP}окт${NBSP}2026 01:30`,
    );
    expect(formatDateTime("nope", "UTC")).toBe("—");
  });
});
