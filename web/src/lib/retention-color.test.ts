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

  // Цвет текста ячейки задаётся токеном темы (--ret-fg-N): в светлой теме тёмный текст на светлых
  // шагах и белый на тёмных, в тёмной — наоборот на самом ярком шаге. Контраст проверяет globals.test.ts.
  it.each([0, 1, 2, 3, 4, 5] as const)("step %s uses themed background and text tokens", (step) => {
    expect(RETENTION_STEP_CLASSES[step]).toBe(`bg-ret-${step} text-(--ret-fg-${step})`);
  });
});
