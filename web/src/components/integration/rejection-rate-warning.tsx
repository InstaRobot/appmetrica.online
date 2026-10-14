import { TriangleAlert } from "lucide-react";

// Порог предупреждения доли отказов (14-ux-ui-spec.md S-10): 1 % и выше.
export const REJECTION_WARNING_RATE = 0.01;

export function isRejectionRateHigh(rejected: number, accepted: number): boolean {
  const total = accepted + rejected;
  return total > 0 && rejected / total >= REJECTION_WARNING_RATE;
}

export function RejectionRateWarning() {
  return (
    <TriangleAlert
      role="img"
      aria-label="Доля отказов 1 % и выше"
      className="size-4 shrink-0 text-warning"
    />
  );
}
