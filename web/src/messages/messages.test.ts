import { describe, expect, it } from "vitest";
import type { Plural } from "@/lib/i18n/format-message";
import { en } from "./en";
import { getMessages } from "./index";
import { ru } from "./ru";

type Leaf = { path: string; value: string | Plural };

function leaves(node: unknown, path = ""): Leaf[] {
  if (typeof node === "string") return [{ path, value: node }];
  if (isPlural(node)) return [{ path, value: node }];
  if (typeof node !== "object" || node === null) throw new Error(`${path}: не строка и не объект`);
  return Object.entries(node).flatMap(([key, child]) =>
    leaves(child, path ? `${path}.${key}` : key),
  );
}

function isPlural(node: unknown): node is Plural {
  return typeof node === "object" && node !== null && "one" in node && "other" in node;
}

const placeholders = (value: string | Plural) =>
  [
    ...new Set(
      (typeof value === "string" ? [value] : Object.values(value)).flatMap(
        (s) => s.match(/\{\w+\}/g) ?? [],
      ),
    ),
  ].sort();

const enLeaves = leaves(en);
const ruLeaves = new Map(leaves(ru).map((l) => [l.path, l.value]));

// Типы требуют одинаковые ключи; тест ловит то, чего типы не видят: формы плюрала, плейсхолдеры, пустые и
// непереведённые строки.
describe("messages", () => {
  it("has the same keys in English and Russian", () => {
    expect([...ruLeaves.keys()].sort()).toEqual(enLeaves.map((l) => l.path).sort());
  });

  it("keeps the same placeholders in both languages", () => {
    for (const { path, value } of enLeaves) {
      const other = ruLeaves.get(path);
      if (other === undefined) continue;
      expect(placeholders(other), path).toEqual(placeholders(value));
    }
  });

  it("gives Russian plurals the few and many forms", () => {
    for (const [path, value] of ruLeaves) {
      if (typeof value === "string") continue;
      expect(value.few, `${path}.few`).toBeTruthy();
      expect(value.many, `${path}.many`).toBeTruthy();
    }
  });

  it("has no empty strings and no Cyrillic in English", () => {
    for (const { path, value } of enLeaves) {
      for (const text of typeof value === "string" ? [value] : Object.values(value)) {
        expect(text.trim(), path).not.toBe("");
        expect(text, path).not.toMatch(/[А-Яа-яЁё]/);
      }
    }
    for (const [path, value] of ruLeaves) {
      for (const text of typeof value === "string" ? [value] : Object.values(value)) {
        expect(text.trim(), path).not.toBe("");
      }
    }
  });

  it("returns the dictionary of the requested locale", () => {
    expect(getMessages("en")).toBe(en);
    expect(getMessages("ru")).toBe(ru);
  });
});
