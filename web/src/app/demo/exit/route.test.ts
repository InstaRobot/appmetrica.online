// @vitest-environment node
import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";
import { GET } from "./route";

describe("GET /demo/exit", () => {
  it("turns demo mode off and returns to the landing", async () => {
    const res = await GET(
      new NextRequest("http://localhost:3000/demo/exit", { headers: { cookie: "am_demo=1" } }),
    );

    expect(res.status).toBe(307);
    expect(res.headers.get("location")).toBe("http://localhost:3000/");
    expect(res.headers.get("set-cookie") ?? "").toMatch(/^am_demo=;.*Max-Age=0/i);
  });
});
