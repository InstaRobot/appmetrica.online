import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { IntegrationHealth } from "@/lib/api/gen/model";
import { formatInt, formatRejectionRate, formatTime } from "@/lib/format";
import { isRejectionRateHigh, RejectionRateWarning } from "./rejection-rate-warning";

type VersionsHealthTableProps = { versions: IntegrationHealth["versions"]; timezone: string };

// Здоровье по app_version за 24 ч (14-ux-ui-spec.md S-10): доля отказов ≥ 1 % помечается ⚠.
export function VersionsHealthTable({ versions, timezone }: VersionsHealthTableProps) {
  if (versions.length === 0) {
    return <p className="text-body-md text-fg-muted">Событий за 24 часа нет</p>;
  }

  return (
    <Table>
      <caption className="sr-only">По версиям приложения</caption>
      <TableHeader>
        <TableRow>
          <TableHead>Версия</TableHead>
          <TableHead className="text-right">Принято</TableHead>
          <TableHead className="text-right">Отклонено</TableHead>
          <TableHead className="text-right">Доля отказов</TableHead>
          <TableHead className="text-right">Последнее событие</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {versions.map((v) => (
          <TableRow key={v.app_version}>
            <TableCell className="font-mono text-code">
              {v.app_version === "" ? (
                <span className="font-sans text-fg-muted">(не задана)</span>
              ) : (
                v.app_version
              )}
            </TableCell>
            <TableCell className="text-right tabular-nums">{formatInt(v.accepted)}</TableCell>
            <TableCell className="text-right tabular-nums">{formatInt(v.rejected)}</TableCell>
            <TableCell className="text-right tabular-nums">
              <span className="inline-flex items-center justify-end gap-1">
                {formatRejectionRate(v.rejected, v.accepted)}
                {isRejectionRateHigh(v.rejected, v.accepted) ? <RejectionRateWarning /> : null}
              </span>
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {formatTime(v.last_seen, timezone)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
