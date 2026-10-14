// Последовательная 6-шаговая шкала тепловой карты retention (14-ux-ui-spec.md «Retention heatmap»):
// границы 5/10/20/35/60 %, тёмный текст на светлых шагах и белый на тёмных — контраст не ниже AA.
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
  0: "bg-ret-0 text-foreground",
  1: "bg-ret-1 text-foreground",
  2: "bg-ret-2 text-foreground",
  3: "bg-ret-3 text-(--color-neutral-950)",
  4: "bg-ret-4 text-white",
  5: "bg-ret-5 text-white",
};
