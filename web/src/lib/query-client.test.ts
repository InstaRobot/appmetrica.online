import { describe, expect, it } from "vitest";
import { ApiError } from "./api/fetcher";
import { makeQueryClient, retryDelay, shouldRetry } from "./query-client";

const apiError = (status: number, code: string) => new ApiError(status, code, code, "r");

describe("query client", () => {
  it("uses 30 s staleTime and the retry policy", () => {
    const client = makeQueryClient();
    const defaults = client.getDefaultOptions().queries;

    expect(defaults?.staleTime).toBe(30_000);
    expect(defaults?.retry).toBe(shouldRetry);
    expect(defaults?.retryDelay).toBe(retryDelay);
    expect(client.getDefaultOptions().mutations?.retry).toBe(false);
  });

  it("does not retry 4xx", () => {
    for (const status of [400, 401, 403, 404, 409, 422, 429]) {
      expect(shouldRetry(0, apiError(status, "x"))).toBe(false);
    }
  });

  it("retries other errors once", () => {
    expect(shouldRetry(0, apiError(500, "internal_error"))).toBe(true);
    expect(shouldRetry(1, apiError(500, "internal_error"))).toBe(false);
    expect(shouldRetry(0, apiError(0, "network_error"))).toBe(true);
    expect(shouldRetry(0, new Error("boom"))).toBe(true);
    expect(shouldRetry(1, new Error("boom"))).toBe(false);
  });

  it("retries storage_unavailable three times with 1, 2, 4 s backoff", () => {
    const err = apiError(503, "storage_unavailable");
    expect([0, 1, 2, 3].map((n) => shouldRetry(n, err))).toEqual([true, true, true, false]);
    expect([0, 1, 2].map((n) => retryDelay(n))).toEqual([1000, 2000, 4000]);
  });
});
