// @vitest-environment node
import { describe, expect, it } from "vitest";
import { GET } from "./route";

describe("GET /demo/exit", () => {
  it("turns demo mode off and returns to the landing", async () => {
    const res = GET();

    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toBe("/");
    expect(res.headers.get("set-cookie") ?? "").toMatch(/^am_demo=;.*Max-Age=0/i);
  });
});
