// Последовательная 6-шаговая шкала тепловой карты retention (14-ux-ui-spec.md «Retention heatmap»):
// границы 5/10/20/35/60 %. Цвет текста ячейки — токен темы --ret-fg-N (globals.css): контраст не ниже AA
// в светлой и тёмной темах проверяет globals.test.ts.
export type RetentionStep = 0 | 1 | 2 | 3 | 4 | 5;

const THRESHOLDS = [0.05, 0.1, 0.2, 0.35, 0.6] as const;

export function retentionStep(value: number): RetentionStep {
  let step = 0;
  for (const threshold of THRESHOLDS) {
    if (value >= threshold) step++;
  }
  return step as RetentionStep;
}

export const RETENTION_STEP_CLASSES: Record<RetentionStep, string> = {
  0: "bg-ret-0 text-(--ret-fg-0)",
  1: "bg-ret-1 text-(--ret-fg-1)",
  2: "bg-ret-2 text-(--ret-fg-2)",
  3: "bg-ret-3 text-(--ret-fg-3)",
  4: "bg-ret-4 text-(--ret-fg-4)",
  5: "bg-ret-5 text-(--ret-fg-5)",
};
