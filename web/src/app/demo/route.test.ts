// @vitest-environment node
import { describe, expect, it } from "vitest";
import { GET } from "./route";

describe("GET /demo", () => {
  it("turns demo mode on and opens the portfolio", async () => {
    const res = GET();

    expect(res.status).toBe(307);
    // Относительный адрес: за обратным прокси (Caddy, Traefik) request.url standalone-сборки указывает
    // на внутренний 0.0.0.0:3000, а браузер сам разрешит путь относительно публичного домена.
    expect(res.headers.get("location")).toBe("/portfolio");
    const cookie = res.headers.get("set-cookie") ?? "";
    expect(cookie).toMatch(/^am_demo=1;/);
    expect(cookie).toMatch(/Path=\//);
    expect(cookie).toMatch(/SameSite=lax/i);
    // Клиентский fetcher читает признак демо из document.cookie — HttpOnly здесь нельзя.
    expect(cookie).not.toMatch(/HttpOnly/i);
  });
});
