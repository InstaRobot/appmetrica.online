import { createLoader } from "nuqs";
import { afterEach, describe, expect, it, vi } from "vitest";
import { audienceCurl, buildCurl, READ_API_URL, readApiBaseUrl } from "./copy-as-curl";
import { audienceSearchParams, resolveAudienceState } from "./url-state";

const loadAudienceSearch = createLoader(audienceSearchParams);

const APP = "0192aaaa-0000-7000-8000-000000000001";
const AUTH = `-H "Authorization: Bearer $APPMETRICA_SK"`;

const state = (search: string) => resolveAudienceState(loadAudienceSearch(search), "2026-10-13");

afterEach(() => {
  vi.unstubAllEnvs();
});

describe("copy-as-curl", () => {
  it("builds curl for current report", () => {
    // Период 30 дней, platform=ios, environment=production.
    expect(audienceCurl(APP, state("?period=30d&platform=ios"))).toBe(
      `curl ${AUTH} "https://api.appmetrica.online/v1/apps/${APP}/audience/summary?from=2026-09-14&to=2026-10-13&platform=ios&environment=production&unit=persons"`,
    );
  });

  it("builds curl for the chart with granularity, metric and comparison", () => {
    expect(
      audienceCurl(
        APP,
        state("?period=7d&g=hour&metric=sessions&compare=1&app_version=2.1.0&country=RU"),
        "timeseries",
      ),
    ).toBe(
      `curl ${AUTH} "https://api.appmetrica.online/v1/apps/${APP}/audience/timeseries?from=2026-10-07&to=2026-10-13&granularity=hour&metric=sessions&compare=previous_period&app_version=2.1.0&country=RU&environment=production&unit=persons"`,
    );
    expect(audienceCurl(APP, state("?period=7d"), "timeseries")).toContain(
      "granularity=day&metric=active&compare=none&environment",
    );
  });

  it("builds curl for rolling active and breakdown", () => {
    expect(audienceCurl(APP, state("?period=yesterday&env=development"), "rolling-active")).toBe(
      `curl ${AUTH} "https://api.appmetrica.online/v1/apps/${APP}/audience/rolling-active?from=2026-10-12&to=2026-10-12&environment=development&unit=persons"`,
    );
    expect(audienceCurl(APP, state("?dim=country&bmetric=new"), "breakdown")).toBe(
      `curl ${AUTH} "https://api.appmetrica.online/v1/apps/${APP}/audience/breakdown?from=2026-10-07&to=2026-10-13&dimension=country&metric=new&environment=production&unit=persons"`,
    );
  });

  it("percent-encodes values so the shell string stays safe", () => {
    const curl = audienceCurl(APP, state('?app_version=1.0 "beta"$`x`&os_name=a%26b'));
    expect(curl).toContain("app_version=1.0+%22beta%22%24%60x%60&os_name=a%26b");
    expect(curl.slice(curl.indexOf('" "') + 3, -1)).not.toMatch(/["$`\\]/);
  });

  it("uses NEXT_PUBLIC_API_URL when set", () => {
    expect(readApiBaseUrl()).toBe(READ_API_URL);
    vi.stubEnv("NEXT_PUBLIC_API_URL", "http://localhost:8080/");
    expect(readApiBaseUrl()).toBe("http://localhost:8080");
    expect(buildCurl("/v1/apps")).toBe(`curl ${AUTH} "http://localhost:8080/v1/apps"`);
  });
});
