import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { config } from "../proxy";

const dir = import.meta.dirname;
const css = readFileSync(join(dir, "globals.css"), "utf8");

// Первое объявление токена — светлая тема (:root).
function token(name: string): string {
  const value = css.match(new RegExp(`--${name}:\\s*(#[0-9a-f]{6})`, "i"))?.[1];
  if (!value) throw new Error(`no --${name} in globals.css`);
  return value.toLowerCase();
}

// Иконки сайта (Next.js file conventions): icon.svg — вкладка браузера, apple-icon.png — экран «Домой»
// на iOS, favicon.ico — старые браузеры и сервисы, которые запрашивают /favicon.ico напрямую.
describe("site icons", () => {
  it("draws the logo mark in icon.svg with the brand gradient", () => {
    const svg = readFileSync(join(dir, "icon.svg"), "utf8");

    expect(svg).toContain('viewBox="0 0 24 24"');
    const stops = [...svg.matchAll(/stop-color="(#[0-9a-f]{6})"/gi)].map((m) => m[1]?.toLowerCase());
    expect(stops).toEqual([token("primary-top"), token("primary-hover")]);
    expect(svg.match(/<rect /g)).toHaveLength(4); // фон и три столбика
  });

  it("ships a 180×180 apple-icon.png", () => {
    const png = readFileSync(join(dir, "apple-icon.png"));

    expect(png.subarray(1, 4).toString("ascii")).toBe("PNG");
    expect([png.readUInt32BE(16), png.readUInt32BE(20)]).toEqual([180, 180]);
  });

  it("ships favicon.ico with 16 and 32 px images", () => {
    const ico = readFileSync(join(dir, "favicon.ico"));

    expect([ico.readUInt16LE(0), ico.readUInt16LE(2)]).toEqual([0, 1]);
    const count = ico.readUInt16LE(4);
    const sizes = Array.from({ length: count }, (_, i) => ico.readUInt8(6 + i * 16));
    expect(sizes).toEqual([16, 32]);
  });

  it("serves the icons to guests: the auth proxy skips them", () => {
    const matcher = new RegExp(`^${config.matcher[0]}$`);
    // Поэтому иконки — статические файлы: маршрут без расширения (icon.tsx → /icon) прокси отправил бы на вход.
    for (const path of ["/icon.svg", "/apple-icon.png", "/favicon.ico"]) {
      expect(matcher.test(path), path).toBe(false);
    }
  });
});
