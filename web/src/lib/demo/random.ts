// Детерминированный ГПСЧ демо-датасета: одинаковый ключ (приложение, дата, метрика…) → одинаковое число.
// FNV-1a с финальным перемешиванием murmur3 даёт seed, mulberry32 — поток значений [0, 1).

export type SeedPart = string | number;

export function hash32(key: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < key.length; i++) {
    h ^= key.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  h ^= h >>> 16;
  h = Math.imul(h, 0x85ebca6b);
  h ^= h >>> 13;
  h = Math.imul(h, 0xc2b2ae35);
  h ^= h >>> 16;
  return h >>> 0;
}

export function mulberry32(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export function seeded(...parts: SeedPart[]): () => number {
  return mulberry32(hash32(parts.join("|")));
}

export function rand(...parts: SeedPart[]): number {
  return seeded(...parts)();
}

// jitter — множитель 1 ± spread/2 по ключу.
export function jitter(spread: number, ...parts: SeedPart[]): number {
  return 1 + spread * (rand(...parts) - 0.5);
}

// Стохастическое округление: в среднем сохраняет дробные ожидания маленьких счётчиков.
export function roundRandom(value: number, ...parts: SeedPart[]): number {
  return Math.max(0, Math.floor(value + rand("round", ...parts)));
}

// uuidFrom — валидный UUID v4 (RFC 9562: версия 4, вариант 10xx), детерминированный по ключу.
export function uuidFrom(...parts: SeedPart[]): string {
  const next = seeded("uuid", ...parts);
  let hex = "";
  for (let i = 0; i < 32; i++) hex += Math.floor(next() * 16).toString(16);
  const variant = "89ab".charAt(Math.floor(next() * 4));
  hex = `${hex.slice(0, 12)}4${hex.slice(13, 16)}${variant}${hex.slice(17)}`;
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

const ALNUM = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";

export function alnumFrom(length: number, ...parts: SeedPart[]): string {
  const next = seeded("alnum", ...parts);
  let out = "";
  for (let i = 0; i < length; i++) out += ALNUM.charAt(Math.floor(next() * ALNUM.length));
  return out;
}

export function hexFrom(length: number, ...parts: SeedPart[]): string {
  const next = seeded("hex", ...parts);
  let out = "";
  for (let i = 0; i < length; i++) out += Math.floor(next() * 16).toString(16);
  return out;
}

export type Weighted<T> = readonly (readonly [T, number])[];

// pickWeighted — значение из непустого списка весов по r ∈ [0, 1).
export function pickWeighted<T>(items: Weighted<T>, r: number): T {
  const total = items.reduce((sum, [, w]) => sum + w, 0);
  let left = r * total;
  for (const [value, w] of items) {
    left -= w;
    if (left < 0) return value;
  }
  const last = items.at(-1);
  if (!last) throw new Error("pickWeighted: empty list");
  return last[0];
}

// splitTotal — целые доли total пропорционально весам (метод наибольшего остатка), сумма ровно total.
export function splitTotal(total: number, weights: readonly number[]): number[] {
  const sum = weights.reduce((s, w) => s + w, 0);
  if (sum <= 0 || total <= 0) return weights.map(() => 0);
  const exact = weights.map((w) => (total * w) / sum);
  const out = exact.map(Math.floor);
  let rest = total - out.reduce((s, v) => s + v, 0);
  const order = exact
    .map((v, i) => ({ i, frac: v - Math.floor(v) }))
    .sort((a, b) => b.frac - a.frac || a.i - b.i);
  for (const { i } of order) {
    if (rest <= 0) break;
    out[i] = (out[i] ?? 0) + 1;
    rest--;
  }
  return out;
}
