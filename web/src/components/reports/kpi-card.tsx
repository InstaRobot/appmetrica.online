"use client";

import { Info } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import type { Formatters } from "@/lib/format";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { cn } from "@/lib/utils";
import type { Messages } from "@/messages";
import { Delta } from "./delta";
import { Sparkline } from "./sparkline";

export type KpiCardProps = {
  label: string;
  /** Текст tooltip из словаря metrics. */
  definition: string;
  value: number | null;
  format: "number" | "duration";
  /** Из AudienceSummary.compare.delta_pct; undefined/null — дельта не показывается. */
  deltaPct?: number | null | undefined;
  /** «медиана 2 мин 5 с». */
  secondary?: string | undefined;
  sparkline?: Array<{ ts: string; value: number | null }> | undefined;
  /** Текущая метрика MetricChart. */
  selected?: boolean;
  onSelect?: () => void;
  loading?: boolean;
};

type KpiMessages = Messages["reports"]["kpi"];

// deltaLabel — дельта словами для скринридеров: «рост на 4,1 % к предыдущему периоду».
export function deltaLabel(
  value: number | null,
  delta: KpiMessages["delta"],
  f: Formatters,
): string | null {
  if (value === null) return null;
  const text = f.formatDelta(value);
  const abs = text.replace(/^[+−]/, "");
  if (text.startsWith("+")) return fmt(delta.up, { delta: abs });
  if (text.startsWith("−")) return fmt(delta.down, { delta: abs });
  return delta.flat;
}

function trendLabel(values: number[], trend: KpiMessages["trend"]): string {
  const first = values[0] ?? 0;
  const last = values.at(-1) ?? 0;
  return last > first ? trend.up : last < first ? trend.down : trend.flat;
}

// KPI-карточка S-05 (14-ux-ui-spec.md §16.5): кнопка-переключатель метрики графика, tooltip с определением
// метрики рядом (вложенные интерактивные элементы в кнопке недопустимы).
export function KpiCard({
  label,
  definition,
  value,
  format,
  deltaPct,
  secondary,
  sparkline,
  selected = false,
  onSelect,
  loading = false,
}: KpiCardProps) {
  const messages = useMessages();
  const m = messages.reports.kpi;
  const f = useFormat();
  const points = (sparkline ?? []).flatMap((p) => (p.value === null ? [] : [p.value]));
  const delta = deltaPct === undefined ? null : deltaLabel(deltaPct, m.delta, f);

  return (
    <div
      className={cn(
        "relative min-h-33 rounded-xl border border-border bg-card shadow-xs transition-colors",
        selected && "border-primary ring-1 ring-primary",
      )}
    >
      <button
        type="button"
        aria-pressed={selected}
        onClick={onSelect}
        className="flex h-full w-full flex-col items-start gap-1.5 rounded-xl p-4 text-left outline-none hover:bg-subtle/60 focus-visible:ring-2 focus-visible:ring-ring md:p-5"
      >
        <span data-kpi-label className="pr-8 text-label-md text-fg-muted">
          {label}
        </span>
        {loading ? (
          <span
            role="status"
            aria-label={messages.common.loading}
            className="flex w-full flex-col gap-2"
          >
            <Skeleton className="h-7 w-30" />
            <Skeleton className="h-3.5 w-15" />
            <Skeleton className="h-8 w-full" />
          </span>
        ) : (
          <>
            <span data-kpi-value className="text-heading-2 tabular-nums md:text-heading-1">
              {format === "duration" ? f.formatDuration(value) : f.formatInt(value)}
            </span>
            {delta !== null && deltaPct !== undefined ? (
              <span role="img" aria-label={delta}>
                <Delta value={deltaPct} />
              </span>
            ) : null}
            {secondary ? <span className="text-body-sm text-fg-muted">{secondary}</span> : null}
            {points.length > 1 ? (
              <span className="mt-1 block w-full">
                <Sparkline
                  values={points}
                  width={200}
                  height={32}
                  fluid
                  trendLabel={trendLabel(points, m.trend)}
                />
              </span>
            ) : null}
          </>
        )}
      </button>
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon-xs"
              aria-label={fmt(m.about, { metric: label })}
              className="absolute top-3 right-3 text-fg-muted md:top-4 md:right-4"
            >
              <Info aria-hidden="true" />
            </Button>
          </TooltipTrigger>
          <TooltipContent className="max-w-64">{definition}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  );
}
