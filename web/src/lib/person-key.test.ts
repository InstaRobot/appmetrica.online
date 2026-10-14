import { describe, expect, it } from "vitest";
import {
  decodePersonKey,
  deletionSubject,
  encodePersonKey,
  formatPersonKey,
  parsePersonKey,
  parseSearchQuery,
  profileHref,
} from "./person-key";

const INSTALL = "5d8e2c1a-0b3f-4e5f-9a7b-0c1d2e3f4e5f";

describe("person-key", () => {
  it("encodes and decodes person keys", () => {
    const user = { kind: "user", value: "a/b c" } as const;
    const segment = encodePersonKey(user);
    expect(segment).toBe("u%3Aa%2Fb%20c");
    expect(decodePersonKey(segment)).toEqual(user);

    const install = { kind: "install", value: INSTALL } as const;
    expect(decodePersonKey(encodePersonKey(install))).toEqual(install);
    expect(encodePersonKey("u:42")).toBe("u%3A42");
  });

  it("keeps percent signs and unicode in user_id", () => {
    for (const value of ["50%", "%3A", "u:42", "пользователь №1", "a?b#c&d=e"]) {
      const key = { kind: "user", value } as const;
      expect(decodePersonKey(encodePersonKey(key))).toEqual(key);
    }
  });

  it("accepts a segment that the router has already decoded", () => {
    expect(decodePersonKey("u:a/b c")).toEqual({ kind: "user", value: "a/b c" });
    expect(decodePersonKey("u:50%")).toEqual({ kind: "user", value: "50%" });
    expect(decodePersonKey(`i:${INSTALL}`)).toEqual({ kind: "install", value: INSTALL });
  });

  it("rejects unknown prefixes and malformed segments", () => {
    for (const segment of ["x%3A1", "x:1", "u%3A", "u:", "", "42", "u%3A%E0%A4%A", "U%3A42"]) {
      expect(decodePersonKey(segment), segment).toBeNull();
    }
    expect(parsePersonKey("i:")).toBeNull();
    expect(parsePersonKey("u:42")).toEqual({ kind: "user", value: "42" });
  });

  it("formats keys and profile links", () => {
    expect(formatPersonKey({ kind: "install", value: INSTALL })).toBe(`i:${INSTALL}`);
    expect(profileHref("app-1", { kind: "user", value: "a/b" })).toBe(
      "/apps/app-1/profiles/u%3Aa%2Fb",
    );
    expect(profileHref("app-1", "u:42")).toBe("/apps/app-1/profiles/u%3A42");
  });

  it("maps a person key to the deletion subject", () => {
    expect(deletionSubject({ kind: "user", value: "42" })).toEqual({
      subject_type: "user",
      subject_value: "42",
    });
    expect(deletionSubject({ kind: "install", value: INSTALL })).toEqual({
      subject_type: "install",
      subject_value: INSTALL,
    });
  });

  it("parses a search query with optional prefix", () => {
    expect(parseSearchQuery("  42  ")).toEqual({ value: "42", kind: null });
    expect(parseSearchQuery("u:42")).toEqual({ value: "42", kind: "user" });
    expect(parseSearchQuery(` i:${INSTALL} `)).toEqual({ value: INSTALL, kind: "install" });
    expect(parseSearchQuery("u: 42 ")).toEqual({ value: "42", kind: "user" });
    expect(parseSearchQuery("u:")).toEqual({ value: "", kind: "user" });
    expect(parseSearchQuery("x:1")).toEqual({ value: "x:1", kind: null });
    expect(parseSearchQuery("")).toEqual({ value: "", kind: null });
  });
});
