"use client";

import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import type { Unit } from "@/lib/api/gen/model";
import { useMessages } from "@/lib/i18n/client";

// Индикатор сервисного режима `?unit=installs` (14-ux-ui-spec.md §16.7): в обычном режиме не рендерится.
export function UnitBadge({ unit }: { unit: Unit }) {
  const m = useMessages().reports.unitBadge;
  if (unit !== "installs") return null;
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="outline" tabIndex={0}>
            {m.label}
          </Badge>
        </TooltipTrigger>
        <TooltipContent>{m.hint}</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
