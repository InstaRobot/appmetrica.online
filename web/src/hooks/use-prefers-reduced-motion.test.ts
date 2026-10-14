import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { usePrefersReducedMotion } from "./use-prefers-reduced-motion";

type Listener = (event: { matches: boolean }) => void;

function stubMatchMedia(initial: boolean) {
  const listeners = new Set<Listener>();
  const mql = {
    matches: initial,
    media: "(prefers-reduced-motion: reduce)",
    addEventListener: vi.fn((_: string, l: Listener) => listeners.add(l)),
    removeEventListener: vi.fn((_: string, l: Listener) => listeners.delete(l)),
  };
  const matchMedia = vi.fn(() => mql);
  vi.stubGlobal("matchMedia", matchMedia);
  return {
    matchMedia,
    mql,
    listeners,
    change(matches: boolean) {
      mql.matches = matches;
      for (const l of listeners) l({ matches });
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("usePrefersReducedMotion", () => {
  it("follows the prefers-reduced-motion media query", () => {
    const media = stubMatchMedia(false);
    const { result, unmount } = renderHook(() => usePrefersReducedMotion());

    expect(media.matchMedia).toHaveBeenCalledWith("(prefers-reduced-motion: reduce)");
    expect(result.current).toBe(false);
    act(() => media.change(true));
    expect(result.current).toBe(true);

    unmount();
    expect(media.listeners.size).toBe(0);
  });

  it("is true when reduced motion is already on", () => {
    stubMatchMedia(true);
    const { result } = renderHook(() => usePrefersReducedMotion());
    expect(result.current).toBe(true);
  });

  it("is false without matchMedia", () => {
    vi.stubGlobal("matchMedia", undefined);
    const { result } = renderHook(() => usePrefersReducedMotion());
    expect(result.current).toBe(false);
  });
});
