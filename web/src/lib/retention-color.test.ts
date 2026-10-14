import { describe, expect, it } from "vitest";
import { RETENTION_STEP_CLASSES, retentionStep } from "./retention-color";

describe("retentionStep", () => {
  it.each([
    [0, 0],
    [0.049, 0],
    [0.05, 1],
    [0.099, 1],
    [0.1, 2],
    [0.2, 3],
    [0.349, 3],
    [0.35, 4],
    [0.599, 4],
    [0.6, 5],
    [1, 5],
  ] as const)("maps %s to step %s", (value, step) => {
    expect(retentionStep(value)).toBe(step);
  });

  it("clamps values outside 0..1", () => {
    expect(retentionStep(-0.1)).toBe(0);
    expect(retentionStep(1.2)).toBe(5);
  });

  it("uses dark text on light steps and white text on dark steps for AA contrast", () => {
    expect(RETENTION_STEP_CLASSES[0]).toBe("bg-ret-0 text-foreground");
    expect(RETENTION_STEP_CLASSES[2]).toBe("bg-ret-2 text-foreground");
    expect(RETENTION_STEP_CLASSES[3]).toBe("bg-ret-3 text-(--color-neutral-950)");
    expect(RETENTION_STEP_CLASSES[4]).toBe("bg-ret-4 text-white");
    expect(RETENTION_STEP_CLASSES[5]).toBe("bg-ret-5 text-white");
  });
});
