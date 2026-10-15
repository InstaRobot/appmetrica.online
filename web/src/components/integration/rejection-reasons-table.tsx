"use client";

import { CodeBlock } from "@/components/common/code-block";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { IntegrationHealth, IntegrationReason } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { rejectionCode } from "@/lib/rejection-codes";

type RejectionReasonsTableProps = { reasons: IntegrationHealth["reasons"]; timezone: string };

// Первая колонка закреплена при горизонтальной прокрутке таблицы на телефоне; фон карточки не даёт
// прокручиваемым ячейкам просвечивать под ней. С md таблица помещается, а подсветка строки при наведении
// не должна обрываться на первой ячейке — поэтому только max-md.
const PINNED = "max-md:sticky max-md:left-0 max-md:bg-card";

// prettyJson — pretty-print примера payload; не-JSON (invalid_json) показывается как есть.
function prettyJson(payload: string): string {
  try {
    return JSON.stringify(JSON.parse(payload), null, 2);
  } catch {
    return payload;
  }
}

function ExamplePayload({ reason }: { reason: IntegrationReason }) {
  const m = useMessages().integration.reasonsTable;
  const label = fmt(m.examplePayload, { code: reason.code });
  return (
    <Popover>
      <PopoverTrigger asChild>
        {/* Телефон: цель нажатия 44 px, с md — компактная кнопка 24 px. */}
        <Button
          variant="outline"
          size="xs"
          aria-label={label}
          className="h-11 min-w-11 font-mono md:h-6 md:min-w-0"
        >
          {"{ }"}
        </Button>
      </PopoverTrigger>
      {/* Телефон: поповер не шире экрана и не прижат к его краям. */}
      <PopoverContent
        aria-label={label}
        align="end"
        collisionPadding={16}
        className="flex w-[min(560px,calc(100vw-2rem))] flex-col gap-3"
      >
        {/* context.ip сервер уже вырезал (14-ux-ui-spec.md S-10). */}
        <CodeBlock
          code={prettyJson(reason.example_payload)}
          lang="json"
          ariaLabel={m.examplePayloadJson}
          maxHeight={320}
        />
        <p className="text-body-sm text-fg-muted">{reason.example_message}</p>
      </PopoverContent>
    </Popover>
  );
}

// Причины отказов за 24 ч (14-ux-ui-spec.md S-10): код, описание с подсказкой из lib/rejection-codes.ts,
// количество, время последнего отказа и пример payload.
export function RejectionReasonsTable({ reasons, timezone }: RejectionReasonsTableProps) {
  const m = useMessages().integration;
  const f = useFormat();
  if (reasons.length === 0) {
    return <p className="text-body-md text-positive">{m.reasonsTable.none}</p>;
  }

  return (
    <Table>
      <caption className="sr-only">{m.sections.reasons}</caption>
      <TableHeader>
        <TableRow>
          <TableHead className={PINNED}>{m.reasonsTable.code}</TableHead>
          <TableHead>{m.reasonsTable.description}</TableHead>
          <TableHead className="text-right">{m.reasonsTable.count}</TableHead>
          <TableHead className="text-right">{m.reasonsTable.last}</TableHead>
          <TableHead className="text-right">{m.reasonsTable.example}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {reasons.map((reason) => {
          const text = rejectionCode(reason.code, m.rejectionCodes);
          return (
            <TableRow key={reason.code}>
              <TableCell className={`${PINNED} font-mono text-code`}>{reason.code}</TableCell>
              {/* min-w-56: на узком экране описание не сжимается в столбик — таблица прокручивается. */}
              <TableCell className="min-w-56 whitespace-normal">
                <span className="block">{text.description}</span>
                {text.hint ? (
                  <span className="block text-body-sm text-fg-muted">{text.hint}</span>
                ) : null}
              </TableCell>
              <TableCell className="text-right tabular-nums">{f.formatInt(reason.count)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {f.formatTime(reason.last_seen, timezone)}
              </TableCell>
              <TableCell className="text-right">
                <ExamplePayload reason={reason} />
              </TableCell>
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
