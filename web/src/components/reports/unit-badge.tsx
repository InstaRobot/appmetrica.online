"use client";

import { Badge } from "@/components/ui/badge";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip";
import type { Unit } from "@/lib/api/gen/model";

// Индикатор сервисного режима `?unit=installs` (14-ux-ui-spec.md §16.7): в обычном режиме не рендерится.
export function UnitBadge({ unit }: { unit: Unit }) {
  if (unit !== "installs") return null;
  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Badge variant="outline" tabIndex={0}>
            Установки
          </Badge>
        </TooltipTrigger>
        <TooltipContent>
          Сервисный режим: считаются установки, а не люди. Уберите ?unit=installs из адреса, чтобы
          вернуться.
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}
