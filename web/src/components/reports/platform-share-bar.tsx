"use client";

import { platformChartVar, platformLabel } from "@/components/common/platform-icon";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import type { PlatformActive } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";

type PlatformShareBarProps = {
  /** Активные по платформам за день (PortfolioRow.platforms), по убыванию. */
  items: PlatformActive[];
  /** Активные за день без разбивки (active_date): меньше суммы, если люди есть на нескольких платформах. */
  total: number;
};

// Доли платформ в строке портфеля: полоса 96×8, доли — от суммы по платформам,
// потому что человек на двух платформах учтён в обеих. Tooltip с абсолютами открывается и по фокусу.
export function PlatformShareBar({ items, total }: PlatformShareBarProps) {
  const m = useMessages().portfolio.shares;
  const f = useFormat();
  const sum = items.reduce((s, item) => s + item.active, 0);
  if (sum === 0) return <span className="text-fg-muted">—</span>;

  const shares = items.map((item) => ({ ...item, share: item.active / sum }));
  const label = shares
    .map((s) => `${platformLabel(s.platform)} ${f.formatShare(s.share)}`)
    .join(", ");
  const absolutes = items.map((i) => `${platformLabel(i.platform)} ${f.formatInt(i.active)}`);

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <div
            role="img"
            aria-label={label}
            // biome-ignore lint/a11y/noNoninteractiveTabindex: фокус открывает tooltip с абсолютами
            tabIndex={0}
            className="ml-auto flex h-2 w-24 overflow-hidden rounded-full bg-muted outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
          >
            {shares.map((s) => (
              <span
                key={s.platform}
                data-platform={s.platform}
                className="h-full"
                style={{
                  width: `${s.share * 100}%`,
                  backgroundColor: platformChartVar(s.platform),
                }}
              />
            ))}
          </div>
        </TooltipTrigger>
        <TooltipContent>
          {absolutes.join(" · ")}
          {sum > total ? `. ${m.overlap}` : null}
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
