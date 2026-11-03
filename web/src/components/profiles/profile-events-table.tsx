"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { usePlatformLabel } from "@/components/common/platform-icon";
import { ErrorState } from "@/components/feedback/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  getListProfileEventsQueryKey,
  listProfileEvents,
} from "@/lib/api/gen/endpoints/profiles/profiles";
import type { ProfileEvent } from "@/lib/api/gen/model";
import { useFormat, useLocale, useMessages } from "@/lib/i18n/client";
import { plural } from "@/lib/i18n/format-message";

// Событий на страницу (limit): значение по умолчанию API, зафиксированное явно ради ключа кэша.
const EVENTS_LIMIT = 50;
// Сколько свойств видно до раскрытия «ещё N».
const VISIBLE_PROPERTIES = 3;

type ProfileEventsTableProps = {
  appId: string;
  /** person_key в виде сегмента пути (encodePersonKey). */
  personKey: string;
  timezone: string;
};

// Ключи и значения свойств показываются как есть, без перевода: это данные приложения клиента.
function Properties({ properties }: { properties: ProfileEvent["properties"] }) {
  const t = useMessages().profiles.events;
  const locale = useLocale();
  const [expanded, setExpanded] = useState(false);
  const entries = Object.entries(properties);
  if (entries.length === 0) return <span className="text-fg-muted">—</span>;
  const hidden = entries.length - VISIBLE_PROPERTIES;
  const shown = expanded || hidden <= 0 ? entries : entries.slice(0, VISIBLE_PROPERTIES);
  return (
    <div className="flex flex-col items-start gap-1">
      <ul className="flex flex-col gap-0.5 font-mono text-code">
        {shown.map(([key, value]) => (
          <li key={key} className="break-all">{`${key}: ${value}`}</li>
        ))}
      </ul>
      {hidden > 0 ? (
        <Button
          variant="link"
          size="sm"
          className="h-auto p-0"
          aria-expanded={expanded}
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? t.lessProperties : plural(locale, t.moreProperties, hidden)}
        </Button>
      ) : null}
    </div>
  );
}

// Собственные события человека по всем его устройствам (FR-102): новые сверху, «Показать ещё» —
// следующая страница по before. Ошибка остаётся внутри раздела и не ломает остальной профиль.
export function ProfileEventsTable({ appId, personKey, timezone }: ProfileEventsTableProps) {
  const t = useMessages().profiles.events;
  const platformLabel = usePlatformLabel();
  const { formatDateTime } = useFormat();
  const events = useInfiniteQuery({
    queryKey: [...getListProfileEventsQueryKey(appId, personKey, { limit: EVENTS_LIMIT }), "pages"],
    queryFn: ({ pageParam, signal }) =>
      listProfileEvents(
        appId,
        personKey,
        { limit: EVENTS_LIMIT, ...(pageParam ? { before: pageParam } : {}) },
        { signal },
      ),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => (last.status === 200 ? last.data.next_before : null),
  });

  if (events.isPending) return <Skeleton className="h-24 w-full" />;
  if (events.isError) {
    return <ErrorState error={events.error} onRetry={() => void events.refetch()} />;
  }

  const rows = events.data.pages.flatMap((p) => (p.status === 200 ? p.data.events : []));
  if (rows.length === 0) return <p className="text-body-md text-fg-muted">{t.empty}</p>;

  return (
    <div className="flex flex-col gap-3">
      <Table stackOnMobile>
        <caption className="sr-only">{t.caption}</caption>
        <TableHeader>
          <TableRow>
            <TableHead>{t.columns.time}</TableHead>
            <TableHead>{t.columns.name}</TableHead>
            <TableHead>{t.columns.properties}</TableHead>
            <TableHead>{t.columns.platform}</TableHead>
            <TableHead>{t.columns.version}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((e, i) => (
            // Нет уникального id события: время, сессия и порядок в ленте дают стабильный ключ.
            // biome-ignore lint/suspicious/noArrayIndexKey: события не переставляются, лента только дописывается
            <TableRow key={`${e.event_time}-${e.session_id}-${i}`}>
              <TableCell label={t.columns.time} className="tabular-nums">
                {formatDateTime(e.event_time, timezone)}
              </TableCell>
              <TableCell label={t.columns.name} className="font-mono text-code break-all">
                {e.name}
                {e.environment === "development" ? (
                  <Badge variant="outline" className="ml-2 align-middle font-sans text-fg-muted">
                    {t.developmentBadge}
                  </Badge>
                ) : null}
              </TableCell>
              <TableCell label={t.columns.properties}>
                <Properties properties={e.properties} />
              </TableCell>
              <TableCell label={t.columns.platform}>{platformLabel(e.platform)}</TableCell>
              <TableCell label={t.columns.version} className="font-mono text-code">
                {e.app_version || "—"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {events.hasNextPage ? (
        <Button
          variant="outline"
          className="self-center max-md:h-11 max-md:self-stretch"
          disabled={events.isFetchingNextPage}
          aria-busy={events.isFetchingNextPage || undefined}
          onClick={() => void events.fetchNextPage()}
        >
          {t.more}
        </Button>
      ) : null}
    </div>
  );
}
