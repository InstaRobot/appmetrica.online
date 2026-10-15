import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { IntegrationHealth } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { isRejectionRateHigh, RejectionRateWarning } from "./rejection-rate-warning";

type VersionsHealthTableProps = { versions: IntegrationHealth["versions"]; timezone: string };

// Колонка версии закреплена при горизонтальной прокрутке таблицы на телефоне (с md таблица помещается,
// а подсветка строки при наведении не должна обрываться на первой ячейке).
const PINNED = "max-md:sticky max-md:left-0 max-md:bg-card";

// Здоровье по app_version за 24 ч (14-ux-ui-spec.md S-10): доля отказов ≥ 1 % помечается ⚠.
export function VersionsHealthTable({ versions, timezone }: VersionsHealthTableProps) {
  const m = useMessages().integration;
  const f = useFormat();
  if (versions.length === 0) {
    return <p className="text-body-md text-fg-muted">{m.versionsTable.none}</p>;
  }

  return (
    <Table>
      <caption className="sr-only">{m.sections.versions}</caption>
      <TableHeader>
        <TableRow>
          <TableHead className={PINNED}>{m.versionsTable.version}</TableHead>
          <TableHead className="text-right">{m.versionsTable.accepted}</TableHead>
          <TableHead className="text-right">{m.versionsTable.rejected}</TableHead>
          <TableHead className="text-right">{m.versionsTable.rejectionRate}</TableHead>
          <TableHead className="text-right">{m.versionsTable.lastEvent}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {versions.map((v) => (
          <TableRow key={v.app_version}>
            <TableCell className={`${PINNED} font-mono text-code`}>
              {v.app_version === "" ? (
                <span className="font-sans text-fg-muted">{m.versionsTable.notSet}</span>
              ) : (
                v.app_version
              )}
            </TableCell>
            <TableCell className="text-right tabular-nums">{f.formatInt(v.accepted)}</TableCell>
            <TableCell className="text-right tabular-nums">{f.formatInt(v.rejected)}</TableCell>
            <TableCell className="text-right tabular-nums">
              <span className="inline-flex items-center justify-end gap-1">
                {f.formatRejectionRate(v.rejected, v.accepted)}
                {isRejectionRateHigh(v.rejected, v.accepted) ? <RejectionRateWarning /> : null}
              </span>
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {f.formatTime(v.last_seen, timezone)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
