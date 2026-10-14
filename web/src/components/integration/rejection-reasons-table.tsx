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
import { formatInt, formatTime } from "@/lib/format";
import { rejectionCode } from "@/lib/rejection-codes";

type RejectionReasonsTableProps = { reasons: IntegrationHealth["reasons"]; timezone: string };

// prettyJson — pretty-print примера payload; не-JSON (invalid_json) показывается как есть.
function prettyJson(payload: string): string {
  try {
    return JSON.stringify(JSON.parse(payload), null, 2);
  } catch {
    return payload;
  }
}

function ExamplePayload({ reason }: { reason: IntegrationReason }) {
  const label = `Пример payload: ${reason.code}`;
  return (
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline" size="xs" aria-label={label} className="font-mono">
          {"{ }"}
        </Button>
      </PopoverTrigger>
      <PopoverContent aria-label={label} align="end" className="flex w-[560px] flex-col gap-3">
        {/* context.ip сервер уже вырезал (14-ux-ui-spec.md S-10). */}
        <CodeBlock
          code={prettyJson(reason.example_payload)}
          lang="json"
          ariaLabel="Пример payload в JSON"
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
  if (reasons.length === 0) {
    return <p className="text-body-md text-positive">Отказов за 24 часа нет ✓</p>;
  }

  return (
    <Table>
      <caption className="sr-only">Причины отказов</caption>
      <TableHeader>
        <TableRow>
          <TableHead>Код</TableHead>
          <TableHead>Описание</TableHead>
          <TableHead className="text-right">Кол-во</TableHead>
          <TableHead className="text-right">Последний</TableHead>
          <TableHead className="text-right">Пример</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {reasons.map((reason) => {
          const text = rejectionCode(reason.code);
          return (
            <TableRow key={reason.code}>
              <TableCell className="font-mono text-code">{reason.code}</TableCell>
              <TableCell className="whitespace-normal">
                <span className="block">{text.description}</span>
                {text.hint ? (
                  <span className="block text-body-sm text-fg-muted">{text.hint}</span>
                ) : null}
              </TableCell>
              <TableCell className="text-right tabular-nums">{formatInt(reason.count)}</TableCell>
              <TableCell className="text-right tabular-nums">
                {formatTime(reason.last_seen, timezone)}
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
