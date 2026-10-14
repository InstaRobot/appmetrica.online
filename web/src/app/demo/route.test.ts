// @vitest-environment node
import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";
import { GET } from "./route";

describe("GET /demo", () => {
  it("turns demo mode on and opens the portfolio", async () => {
    const res = await GET(new NextRequest("http://localhost:3000/demo"));

    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toBe("http://localhost:3000/portfolio");
    const cookie = res.headers.get("set-cookie") ?? "";
    expect(cookie).toMatch(/^am_demo=1;/);
    expect(cookie).toMatch(/Path=\//);
    expect(cookie).toMatch(/SameSite=lax/i);
    // Клиентский fetcher читает признак демо из document.cookie — HttpOnly здесь нельзя.
    expect(cookie).not.toMatch(/HttpOnly/i);
  });
});
