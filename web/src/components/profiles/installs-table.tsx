"use client";

import { PlatformIcon, usePlatformLabel } from "@/components/common/platform-icon";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ProfileInstall } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";

type InstallsTableProps = { installs: ProfileInstall[]; timezone: string };

const DASH = "—";

function place(i: ProfileInstall): string {
  if (!i.country) return DASH;
  return i.city ? `${i.country} · ${i.city}` : i.country;
}

function os(i: ProfileInstall): string {
  return [i.os_name, i.os_version].filter(Boolean).join(" ") || DASH;
}

// Устройства человека (14-ux-ui-spec.md S-09): контекст последнего события установки, даты — в TZ приложения.
export function InstallsTable({ installs, timezone }: InstallsTableProps) {
  const platformLabel = usePlatformLabel();
  const t = useMessages().profiles.installs;
  const { formatDateShort, shortId } = useFormat();
  return (
    <Table stackOnMobile>
      <caption className="sr-only">{t.caption}</caption>
      <TableHeader>
        <TableRow>
          <TableHead>{t.columns.installId}</TableHead>
          <TableHead>{t.columns.platform}</TableHead>
          <TableHead>{t.columns.device}</TableHead>
          <TableHead>{t.columns.os}</TableHead>
          <TableHead>{t.columns.version}</TableHead>
          <TableHead>{t.columns.geo}</TableHead>
          <TableHead className="text-right">{t.columns.firstSeen}</TableHead>
          <TableHead className="text-right">{t.columns.lastSeen}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {installs.map((i) => (
          <TableRow key={i.install_id}>
            <TableCell
              label={t.columns.installId}
              className="font-mono text-code"
              title={i.install_id}
            >
              {shortId(i.install_id)}
            </TableCell>
            <TableCell label={t.columns.platform}>
              <span className="inline-flex items-center gap-1.5">
                <PlatformIcon platform={i.platform} className="size-4" />
                {platformLabel(i.platform)}
              </span>
            </TableCell>
            <TableCell label={t.columns.device}>{i.device_model || DASH}</TableCell>
            <TableCell label={t.columns.os}>{os(i)}</TableCell>
            <TableCell label={t.columns.version} className="font-mono text-code">
              {i.app_version || DASH}
            </TableCell>
            <TableCell label={t.columns.geo}>{place(i)}</TableCell>
            <TableCell label={t.columns.firstSeen} className="text-right tabular-nums">
              {formatDateShort(i.first_seen, timezone)}
            </TableCell>
            <TableCell label={t.columns.lastSeen} className="text-right tabular-nums">
              {formatDateShort(i.last_seen, timezone)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
