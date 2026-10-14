import { describe, expect, it } from "vitest";
import {
  formatDate,
  formatDateShort,
  formatDelta,
  formatDuration,
  formatInt,
  formatPercent,
  formatWeekdayDate,
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
