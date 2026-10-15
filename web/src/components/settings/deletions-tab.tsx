"use client";

import { ErrorState } from "@/components/feedback/error-state";
import { DeletionStatusBadge } from "@/components/profiles/deletion-status-badge";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ApiError } from "@/lib/api/fetcher";
import { useListDeletionRequests } from "@/lib/api/gen/endpoints/admin-system/admin-system";
import { useListApps } from "@/lib/api/gen/endpoints/apps/apps";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { TabHeader } from "./tab-header";

// Вкладка «Удаления» S-12 (FR-019): журнал запросов на удаление по всем приложениям. Удалённое приложение
// уже не в списке — вместо названия сокращённый app_id.
export function DeletionsTab({ timezone }: { timezone: string }) {
  const t = useMessages().settings.deletions;
  const { formatDateTime, shortId } = useFormat();
  const query = useListDeletionRequests();
  const appsQuery = useListApps({ include_archived: true });
  const apps = appsQuery.data?.status === 200 ? appsQuery.data.data.items : [];
  const requests = query.data?.status === 200 ? query.data.data.items : undefined;
  const appName = (id: string) => apps.find((a) => a.id === id)?.name ?? shortId(id);

  return (
    <section aria-labelledby="deletions-title" className="flex flex-col gap-4">
      <TabHeader id="deletions-title" title={t.title} />
      {query.isError && !requests ? (
        <ErrorState
          error={query.error as unknown as ApiError}
          onRetry={() => void query.refetch()}
        />
      ) : !requests ? (
        <TableSkeleton rows={3} />
      ) : (
        <Table>
          <caption className="sr-only">{t.caption}</caption>
          <TableHeader>
            <TableRow>
              <TableHead>{t.columns.app}</TableHead>
              <TableHead>{t.columns.type}</TableHead>
              <TableHead>{t.columns.value}</TableHead>
              <TableHead>{t.columns.source}</TableHead>
              <TableHead>{t.columns.status}</TableHead>
              <TableHead>{t.columns.created}</TableHead>
              <TableHead>{t.columns.completed}</TableHead>
              <TableHead>{t.columns.error}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {requests.length === 0 ? (
              <TableRow>
                <TableCell colSpan={8} className="py-6 text-center text-fg-muted">
                  {t.empty}
                </TableCell>
              </TableRow>
            ) : (
              requests.map((r) => (
                <TableRow key={r.id}>
                  <TableCell>{appName(r.app_id)}</TableCell>
                  <TableCell>{t.subject[r.subject_type]}</TableCell>
                  <TableCell className="font-mono" title={r.subject_value}>
                    {shortId(r.subject_value)}
                  </TableCell>
                  <TableCell>{t.source[r.source]}</TableCell>
                  <TableCell>
                    <DeletionStatusBadge status={r.status} />
                  </TableCell>
                  <TableCell>{formatDateTime(r.requested_at, timezone)}</TableCell>
                  <TableCell>
                    {r.completed_at ? formatDateTime(r.completed_at, timezone) : "—"}
                  </TableCell>
                  <TableCell className="max-w-64 truncate" title={r.error ?? undefined}>
                    {r.error ?? "—"}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      )}
    </section>
  );
}
