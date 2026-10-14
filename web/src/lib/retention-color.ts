export type RetentionStep = 0 | 1 | 2 | 3 | 4 | 5;

export function retentionStep(_value: number): RetentionStep {
  throw new Error("not implemented");
}

export const RETENTION_STEP_CLASSES: Record<RetentionStep, string> = {
  0: "",
  1: "",
  2: "",
  3: "",
  4: "",
  5: "",
};
