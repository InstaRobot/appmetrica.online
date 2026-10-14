import { PlatformIcon, platformLabel } from "@/components/common/platform-icon";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ProfileInstall } from "@/lib/api/gen/model";
import { formatDateShort, shortId } from "@/lib/format";

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
  return (
    <Table>
      <caption className="sr-only">Устройства</caption>
      <TableHeader>
        <TableRow>
          <TableHead>Install ID</TableHead>
          <TableHead>Платформа</TableHead>
          <TableHead>Устройство</TableHead>
          <TableHead>ОС</TableHead>
          <TableHead>Версия</TableHead>
          <TableHead>Гео</TableHead>
          <TableHead className="text-right">Первый</TableHead>
          <TableHead className="text-right">Последний</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {installs.map((i) => (
          <TableRow key={i.install_id}>
            <TableCell className="font-mono text-code" title={i.install_id}>
              {shortId(i.install_id)}
            </TableCell>
            <TableCell>
              <span className="inline-flex items-center gap-1.5">
                <PlatformIcon platform={i.platform} className="size-4" />
                {platformLabel(i.platform)}
              </span>
            </TableCell>
            <TableCell>{i.device_model || DASH}</TableCell>
            <TableCell>{os(i)}</TableCell>
            <TableCell className="font-mono text-code">{i.app_version || DASH}</TableCell>
            <TableCell>{place(i)}</TableCell>
            <TableCell className="text-right tabular-nums">
              {formatDateShort(i.first_seen, timezone)}
            </TableCell>
            <TableCell className="text-right tabular-nums">
              {formatDateShort(i.last_seen, timezone)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
