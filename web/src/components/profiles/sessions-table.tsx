import { platformLabel } from "@/components/common/platform-icon";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ProfileSession } from "@/lib/api/gen/model";
import { formatDateTime, formatDuration, shortId } from "@/lib/format";

type SessionsTableProps = {
  sessions: ProfileSession[];
  timezone: string;
  hasMore: boolean;
  onLoadMore: () => void;
  loadingMore?: boolean;
};

// Лента сессий человека (14-ux-ui-spec.md S-09): новые сверху, «Показать ещё» — следующая страница по before.
export function SessionsTable({
  sessions,
  timezone,
  hasMore,
  onLoadMore,
  loadingMore = false,
}: SessionsTableProps) {
  if (sessions.length === 0) {
    return <p className="text-body-md text-fg-muted">Сессий нет</p>;
  }

  return (
    <div className="flex flex-col gap-3">
      <Table>
        <caption className="sr-only">Сессии</caption>
        <TableHeader>
          <TableRow>
            <TableHead>Начало</TableHead>
            <TableHead>Длительность</TableHead>
            <TableHead>Версия</TableHead>
            <TableHead>Платформа</TableHead>
            <TableHead>Install</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {sessions.map((s) => (
            <TableRow key={s.session_id}>
              <TableCell className="tabular-nums">
                {formatDateTime(s.started_at, timezone)}
              </TableCell>
              <TableCell className="tabular-nums">
                {s.ended_at === null ? (
                  <span className="text-fg-muted">— (нет session_end)</span>
                ) : (
                  formatDuration(s.duration_ms)
                )}
              </TableCell>
              <TableCell className="font-mono text-code">{s.app_version || "—"}</TableCell>
              <TableCell>{platformLabel(s.platform)}</TableCell>
              <TableCell className="font-mono text-code" title={s.install_id}>
                {shortId(s.install_id)}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {hasMore ? (
        <Button
          variant="outline"
          className="self-center"
          disabled={loadingMore}
          aria-busy={loadingMore || undefined}
          onClick={onLoadMore}
        >
          Показать ещё
        </Button>
      ) : null}
    </div>
  );
}
