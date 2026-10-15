import { TriangleAlert } from "lucide-react";
import { useMessages } from "@/lib/i18n/client";

// Порог предупреждения доли отказов (14-ux-ui-spec.md S-10): 1 % и выше.
export const REJECTION_WARNING_RATE = 0.01;

export function isRejectionRateHigh(rejected: number, accepted: number): boolean {
  const total = accepted + rejected;
  return total > 0 && rejected / total >= REJECTION_WARNING_RATE;
}

export function RejectionRateWarning() {
  const label = useMessages().integration.rateWarning;
  return <TriangleAlert role="img" aria-label={label} className="size-4 shrink-0 text-warning" />;
}
