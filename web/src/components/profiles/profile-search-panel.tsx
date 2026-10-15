"use client";

import { SearchX } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { parseAsString, useQueryState } from "nuqs";
import { useEffect, useState } from "react";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { Card } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type { ApiError } from "@/lib/api/fetcher";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { useSearchProfiles } from "@/lib/api/gen/endpoints/profiles/profiles";
import type { ProfileSummary } from "@/lib/api/gen/model";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { type PersonKind, parseSearchQuery, profileHref } from "@/lib/person-key";
import { ProfileSearch } from "./profile-search";

// Длина install_id/user_id в API (searchProfiles q ≤ 128): длиннее — заведомо ничего нет.
const MAX_VALUE_LENGTH = 128;

const MATCHED_BY: Record<PersonKind, ProfileSummary["matched_by"]> = {
  user: "user_id",
  install: "install_id",
};

type ProfileSearchPanelProps = { appId: string };

// S-08 Профили (FR-016): запрос в `?q=`, префикс `u:`/`i:` срезается и оставляет совпадения своего типа.
// Единственный результат поиска, отправленного на этой странице, сразу открывает S-09; поиск из URL
// («Назад», ссылка) показывает таблицу, чтобы не уводить с экрана повторно.
export function ProfileSearchPanel({ appId }: ProfileSearchPanelProps) {
  const m = useMessages().profiles;
  const router = useRouter();
  const app = useGetApp(appId);
  const timezone = app.data?.status === 200 ? app.data.data.timezone : undefined;
  const [q, setQ] = useQueryState(
    "q",
    parseAsString.withDefault("").withOptions({ history: "push" }),
  );
  const [submitted, setSubmitted] = useState<string | null>(null);

  const { value, kind } = parseSearchQuery(q);
  const searchable = value.length > 0 && value.length <= MAX_VALUE_LENGTH;
  const search = useSearchProfiles(appId, { q: value }, { query: { enabled: searchable } });
  const found = search.data?.status === 200 ? search.data.data.items : undefined;
  const items = kind && found ? found.filter((p) => p.matched_by === MATCHED_BY[kind]) : found;

  const single = submitted === q && items?.length === 1 ? items[0] : undefined;
  useEffect(() => {
    if (single) router.push(profileHref(appId, single.person_key));
  }, [single, router, appId]);

  const onSearch = (next: string) => {
    setSubmitted(next);
    void setQ(next);
  };

  return (
    <div className="flex flex-col gap-6">
      <h2 className="text-heading-2">{m.title}</h2>
      <Card className="px-6">
        <ProfileSearch defaultValue={q} onSearch={onSearch} pending={search.isFetching} />
      </Card>
      {value === "" ? null : (
        <Card className="gap-4 px-6">
          <SearchResults
            appId={appId}
            timezone={timezone}
            searchable={searchable}
            items={items}
            error={search.isError ? (search.error as unknown as ApiError) : null}
            onRetry={() => void search.refetch()}
          />
        </Card>
      )}
    </div>
  );
}

type SearchResultsProps = {
  appId: string;
  timezone: string | undefined;
  searchable: boolean;
  items: ProfileSummary[] | undefined;
  error: ApiError | null;
  onRetry: () => void;
};

function SearchResults({ appId, timezone, searchable, items, error, onRetry }: SearchResultsProps) {
  const t = useMessages().profiles.results;
  const { formatDate, formatInt, shortId } = useFormat();
  if (error) {
    return <ErrorState error={error} onRetry={onRetry} />;
  }
  if (searchable && !items) return <TableSkeleton rows={2} />;
  if (!items || items.length === 0) {
    return <EmptyState icon={SearchX} title={t.emptyTitle} description={t.emptyDescription} />;
  }

  return (
    <Table stackOnMobile>
      <caption className="sr-only">{t.caption}</caption>
      <TableHeader>
        <TableRow>
          <TableHead>{t.columns.person}</TableHead>
          <TableHead>{t.columns.userId}</TableHead>
          <TableHead className="text-right">{t.columns.installs}</TableHead>
          <TableHead className="text-right">{t.columns.firstSeen}</TableHead>
          <TableHead className="text-right">{t.columns.lastSeen}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((p) => (
          <TableRow key={p.person_key}>
            <TableCell className="font-mono text-code">
              <Link
                href={profileHref(appId, p.person_key)}
                aria-label={fmt(t.profileLink, { key: p.person_key })}
                title={p.person_key}
                // На телефоне ссылка — заголовок карточки высотой не меньше 44 px.
                className="text-fg-link hover:underline max-md:inline-flex max-md:min-h-11 max-md:items-center"
              >
                {p.person_key.slice(0, 2)}
                {shortId(p.person_key.slice(2))}
              </Link>
            </TableCell>
            <TableCell
              label={t.columns.userId}
              className="font-mono text-code"
              title={p.user_id ?? undefined}
            >
              {p.user_id === null ? "—" : shortId(p.user_id)}
            </TableCell>
            <TableCell label={t.columns.installs} className="text-right tabular-nums">
              {formatInt(p.installs_count)}
            </TableCell>
            <TableCell label={t.columns.firstSeen} className="text-right tabular-nums">
              {formatDate(p.first_seen, timezone)}
            </TableCell>
            <TableCell label={t.columns.lastSeen} className="text-right tabular-nums">
              {formatDate(p.last_seen, timezone)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
