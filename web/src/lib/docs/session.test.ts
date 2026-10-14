import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { getGetAuthStateMockHandler } from "@/lib/api/gen/endpoints/admin-auth/admin-auth.msw";
import { server } from "@/test/msw";
import { isOwner } from "./session";

// Документация публичная: сессия решает только, какую шапку показать над ней.
describe("isOwner", () => {
  it("is true for a valid owner session", async () => {
    server.use(getGetAuthStateMockHandler({ authenticated: true, setup_required: false }));

    await expect(isOwner()).resolves.toBe(true);
  });

  it("is false for a guest", async () => {
    server.use(getGetAuthStateMockHandler({ authenticated: false, setup_required: false }));

    await expect(isOwner()).resolves.toBe(false);
  });

  it("treats an unreachable API as a guest instead of failing the docs", async () => {
    server.use(http.get("*/admin/v1/auth/state", () => HttpResponse.error()));

    await expect(isOwner()).resolves.toBe(false);
  });
});
