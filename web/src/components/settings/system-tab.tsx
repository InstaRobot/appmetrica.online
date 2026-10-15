"use client";

import { keepPreviousData } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { ErrorState } from "@/components/feedback/error-state";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Badge } from "@/components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetSystemStatus } from "@/lib/api/gen/endpoints/admin-system/admin-system";
import type { SystemStatus } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { cn } from "@/lib/utils";
import { TabHeader } from "./tab-header";

const LOW_DISK_PCT = 20;
const BACKUP_STALE_MS = 26 * 60 * 60 * 1000;

function Health({ ok }: { ok: boolean }) {
  const t = useMessages().settings.system;
  return (
    <span className="inline-flex items-center gap-1.5">
      <span
        aria-hidden="true"
        className={cn("size-2 rounded-full", ok ? "bg-success-500" : "bg-danger-500")}
      />
      {ok ? t.ok : t.unavailable}
    </span>
  );
}

function StatusLine({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    // biome-ignore lint/a11y/useSemanticElements: строка статуса компонента, а не набор полей формы
    <div role="group" aria-label={label} className="flex flex-wrap items-center gap-x-2">
      {children}
    </div>
  );
}

function StaleWarning({ children }: { children: React.ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1 text-negative">
      <TriangleAlert aria-hidden="true" className="size-4" />
      {children}
    </span>
  );
}

function Components({
  status,
  timezone,
  now,
}: {
  status: SystemStatus;
  timezone: string;
  now: Date;
}) {
  const t = useMessages().settings.system;
  const f = useFormat();
  const { clickhouse, postgres, backup } = status;
  const lowDisk = clickhouse.disk_free_pct !== null && clickhouse.disk_free_pct < LOW_DISK_PCT;
  const backupAt = backup.last_backup_at ? Date.parse(backup.last_backup_at) : Number.NaN;
  const backupStale = Number.isNaN(backupAt) || now.getTime() - backupAt > BACKUP_STALE_MS;

  return (
    <div className="flex flex-col gap-2 text-body-md">
      <StatusLine label="ClickHouse">
        <span className="font-medium">ClickHouse</span>
        <Health ok={clickhouse.ok} />
        {clickhouse.disk_free_pct !== null ? (
          <span className={cn("inline-flex items-center gap-1", lowDisk && "text-negative")}>
            <span aria-hidden="true">·</span>
            {lowDisk ? <TriangleAlert aria-hidden="true" className="size-4" /> : null}
            {fmt(t.diskFree, { value: f.formatShare(Math.round(clickhouse.disk_free_pct) / 100) })}
          </span>
        ) : null}
        {clickhouse.events_rows !== null ? (
          <span>{fmt(t.events, { count: f.formatInt(clickhouse.events_rows) })}</span>
        ) : null}
      </StatusLine>
      <StatusLine label="Postgres">
        <span className="font-medium">Postgres</span>
        <Health ok={postgres.ok} />
      </StatusLine>
      <StatusLine label={t.backup}>
        <span className="font-medium">{t.backupLabel}</span>
        {backup.last_backup_at ? (
          <span>
            {f.formatDateTime(backup.last_backup_at, timezone)}
            {backup.last_backup_name ? ` (${backup.last_backup_name})` : ""}
          </span>
        ) : (
          <span>{t.noData}</span>
        )}
        {backupStale ? <StaleWarning>{t.backupStale}</StaleWarning> : null}
      </StatusLine>
    </div>
  );
}

function AlertsTable({ status, timezone }: { status: SystemStatus; timezone: string }) {
  const t = useMessages().settings.system;
  const f = useFormat();
  return (
    <Table>
      <caption className="sr-only">{t.alertsCaption}</caption>
      <TableHeader>
        <TableRow>
          <TableHead>{t.alertColumns.alert}</TableHead>
          <TableHead>{t.alertColumns.state}</TableHead>
          <TableHead>{t.alertColumns.value}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {status.alerts.length === 0 ? (
          <TableRow>
            <TableCell colSpan={3} className="text-fg-muted">
              {t.alertsEmpty}
            </TableCell>
          </TableRow>
        ) : (
          status.alerts.map((alert) => (
            <TableRow key={alert.key}>
              <TableCell className="font-mono">{alert.key}</TableCell>
              <TableCell>
                {alert.firing ? (
                  <Badge className="bg-destructive/10 text-destructive">
                    {alert.first_fired_at
                      ? fmt(t.firingSince, { time: f.formatTime(alert.first_fired_at, timezone) })
                      : t.firing}
                  </Badge>
                ) : (
                  <Badge className="bg-success-50 text-positive">{t.normal}</Badge>
                )}
              </TableCell>
              <TableCell className="font-mono">{alert.last_value || "—"}</TableCell>
            </TableRow>
          ))
        )}
      </TableBody>
    </Table>
  );
}

function JobsTable({ status, timezone }: { status: SystemStatus; timezone: string }) {
  const t = useMessages().settings.system;
  const f = useFormat();
  return (
    <Table>
      <caption className="sr-only">{t.jobsCaption}</caption>
      <TableHeader>
        <TableRow>
          <TableHead>{t.jobColumns.job}</TableHead>
          <TableHead>{t.jobColumns.lastRun}</TableHead>
          <TableHead>{t.jobColumns.status}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {status.jobs.length === 0 ? (
          <TableRow>
            <TableCell colSpan={3} className="text-fg-muted">
              {t.jobsEmpty}
            </TableCell>
          </TableRow>
        ) : (
          status.jobs.map((run) => (
            <TableRow key={run.id}>
              <TableCell className="font-mono">{run.job}</TableCell>
              <TableCell>{f.formatDateTime(run.started_at, timezone)}</TableCell>
              <TableCell className={cn("font-mono", run.status === "failed" && "text-negative")}>
                {run.status}
              </TableCell>
            </TableRow>
          ))
        )}
      </TableBody>
    </Table>
  );
}

type SystemTabProps = { timezone: string; now?: Date };

// Вкладка «Система» S-12 (FR-020): ClickHouse, Postgres, последний бэкап, алерты и последние запуски job.
// Обновляется раз в 30 с на месте, без skeleton (keepPreviousData).
export function SystemTab({ timezone, now = new Date() }: SystemTabProps) {
  const m = useMessages();
  const query = useGetSystemStatus({
    query: { refetchInterval: 30_000, placeholderData: keepPreviousData },
  });
  const status = query.data?.status === 200 ? query.data.data : undefined;

  return (
    <section aria-labelledby="system-title" className="flex flex-col gap-4">
      <TabHeader id="system-title" title={m.settings.system.title} />
      {query.isError && !status ? (
        <ErrorState
          error={query.error as unknown as ApiError}
          onRetry={() => void query.refetch()}
        />
      ) : !status ? (
        <TableSkeleton rows={4} />
      ) : (
        <>
          <Components status={status} timezone={timezone} now={now} />
          <div className="grid gap-6 lg:grid-cols-2">
            <AlertsTable status={status} timezone={timezone} />
            <JobsTable status={status} timezone={timezone} />
          </div>
        </>
      )}
    </section>
  );
}
