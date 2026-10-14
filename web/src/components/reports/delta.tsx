import { ArrowDown, ArrowUp } from "lucide-react";
import { formatDelta } from "@/lib/format";
import { cn } from "@/lib/utils";

type DeltaProps = {
  /** Проценты: 4.1 = +4,1 %. null — сравнивать не с чем (показывается «—»). */
  value: number | null;
  /** Для метрик, где рост — плохо (доля отказов на S-10). */
  invert?: boolean;
  size?: "sm" | "md";
};

// Дельта к прошлому периоду (14-ux-ui-spec.md §16.5): знак, стрелка и цвет; ноль и «—» нейтральные.
export function Delta({ value, invert = false, size = "sm" }: DeltaProps) {
  const text = formatDelta(value);
  const sign = text.startsWith("+") ? 1 : text.startsWith("−") ? -1 : 0;
  const good = invert ? sign < 0 : sign > 0;
  const Arrow = sign > 0 ? ArrowUp : ArrowDown;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-0.5 whitespace-nowrap tabular-nums",
        size === "md" ? "text-body-md" : "text-body-sm",
        sign === 0 ? "text-fg-muted" : good ? "text-positive" : "text-negative",
      )}
    >
      {sign !== 0 ? <Arrow aria-hidden="true" className="size-3.5 shrink-0" /> : null}
      {text}
    </span>
  );
}
