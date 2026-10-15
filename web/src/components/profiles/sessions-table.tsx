"use client";

import { usePlatformLabel } from "@/components/common/platform-icon";
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
import { useFormat, useMessages } from "@/lib/i18n/client";

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
  const platformLabel = usePlatformLabel();
  const t = useMessages().profiles.sessions;
  const { formatDateTime, formatDuration, shortId } = useFormat();
  if (sessions.length === 0) {
    return <p className="text-body-md text-fg-muted">{t.empty}</p>;
  }

  return (
    <div className="flex flex-col gap-3">
      <Table>
        <caption className="sr-only">{t.caption}</caption>
        <TableHeader>
          <TableRow>
            <TableHead>{t.columns.start}</TableHead>
            <TableHead>{t.columns.duration}</TableHead>
            <TableHead>{t.columns.version}</TableHead>
            <TableHead>{t.columns.platform}</TableHead>
            <TableHead>{t.columns.install}</TableHead>
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
                  <span className="text-fg-muted">{t.noEnd}</span>
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
          {t.more}
        </Button>
      ) : null}
    </div>
  );
}
