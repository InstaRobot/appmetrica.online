"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { ArrowLeft, UserX } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import { EmptyState } from "@/components/feedback/empty-state";
import { ErrorState } from "@/components/feedback/error-state";
import { ProfileSkeleton } from "@/components/skeletons/profile-skeleton";
import { Card } from "@/components/ui/card";
import { ApiError } from "@/lib/api/fetcher";
import { useListDeletionRequests } from "@/lib/api/gen/endpoints/admin-system/admin-system";
import { useGetApp } from "@/lib/api/gen/endpoints/apps/apps";
import { useGetDeletionRequest } from "@/lib/api/gen/endpoints/deletion/deletion";
import { getGetProfileQueryKey, getProfile } from "@/lib/api/gen/endpoints/profiles/profiles";
import type { App, DeletionRequest } from "@/lib/api/gen/model";
import { decodePersonKey, encodePersonKey, type PersonKey } from "@/lib/person-key";
import { InstallsTable } from "./installs-table";
import { isDeletionActive, ProfileCard } from "./profile-card";
import { ProfileDeleteDialog } from "./profile-delete-dialog";
import { SessionsTable } from "./sessions-table";

// Сессий на страницу (sessions_limit), опрос статуса удаления и пауза перед возвратом к поиску (S-09).
const SESSIONS_LIMIT = 100;
const DELETION_POLL_MS = 10_000;
const REDIRECT_MS = 2000;

type ProfilePanelProps = {
  appId: string;
  /** Сегмент пути `[personKey]`: закодированный или уже раскодированный роутером person_key. */
  personKey: string;
};

const searchHref = (appId: string) => `/apps/${appId}/profiles` as Route;

function NotFound({ appId }: { appId: string }) {
  return (
    <Card className="px-6">
      <EmptyState
        icon={UserX}
        title="Профиль не найден"
        description="Возможно, данные удалены или ещё не поступили."
        action={{ label: "К поиску", href: searchHref(appId) }}
      />
    </Card>
  );
}

// S-09 Профиль (FR-016, FR-019). Приложение (имя, TZ) берётся из кэша AppShell.
export function ProfilePanel({ appId, personKey }: ProfilePanelProps) {
  const key = decodePersonKey(personKey);
  const app = useGetApp(appId);
  if (!key) return <NotFound appId={appId} />;
  return app.data?.status === 200 ? (
    <ProfileView app={app.data.data} personKey={key} />
  ) : (
    <ProfileSkeleton />
  );
}

// latestDeletion — последний запрос журнала по этому person (install_id хранится в нижнем регистре, OQ-057).
function latestDeletion(items: DeletionRequest[], key: PersonKey): DeletionRequest | undefined {
  const value = key.kind === "install" ? key.value.toLowerCase() : key.value;
  let latest: DeletionRequest | undefined;
  for (const r of items) {
    if (r.subject_type !== key.kind || r.subject_value !== value) continue;
    if (!latest || r.requested_at > latest.requested_at) latest = r;
  }
  return latest;
}

// useDeletion — состояние удаления person: последний запрос из журнала или только что созданный,
// пока он в очереди или выполняется — опрос getDeletionRequest раз в 10 с до done/failed.
function useDeletion(appId: string, key: PersonKey) {
  const listed = useListDeletionRequests({ app_id: appId });
  const [created, setCreated] = useState<DeletionRequest>();
  const base =
    created ??
    (listed.data?.status === 200 ? latestDeletion(listed.data.data.items, key) : undefined);

  const polled = useGetDeletionRequest(appId, base?.id ?? "", {
    query: {
      enabled: isDeletionActive(base),
      staleTime: 0,
      refetchInterval: (query) => {
        const res = query.state.data;
        return res?.status === 200 && !isDeletionActive(res.data) ? false : DELETION_POLL_MS;
      },
    },
  });
  const fresh = polled.data?.status === 200 ? polled.data.data : undefined;
  const deletion = fresh && fresh.id === base?.id ? fresh : base;
  return { deletion, setCreated };
}

function ProfileView({ app, personKey }: { app: App; personKey: PersonKey }) {
  const router = useRouter();
  const encoded = encodePersonKey(personKey);
  const profile = useInfiniteQuery({
    queryKey: [
      ...getGetProfileQueryKey(app.id, encoded, { sessions_limit: SESSIONS_LIMIT }),
      "pages",
    ],
    queryFn: ({ pageParam, signal }) =>
      getProfile(
        app.id,
        encoded,
        { sessions_limit: SESSIONS_LIMIT, ...(pageParam ? { before: pageParam } : {}) },
        { signal },
      ),
    initialPageParam: null as string | null,
    getNextPageParam: (last) => (last.status === 200 ? last.data.next_before : null),
  });

  const { deletion, setCreated } = useDeletion(app.id, personKey);
  const [dialogOpen, setDialogOpen] = useState(false);

  // Возврат к поиску — только если запрос был в работе при открытой странице и завершился.
  const [watchedId, setWatchedId] = useState<string | null>(null);
  if (deletion && isDeletionActive(deletion) && watchedId !== deletion.id) {
    setWatchedId(deletion.id);
  }
  const doneId = deletion?.status === "done" && deletion.id === watchedId ? deletion.id : null;
  useEffect(() => {
    if (!doneId) return;
    toast.success("Данные удалены", { id: `deletion-${doneId}` });
    const timer = setTimeout(() => router.push(searchHref(app.id)), REDIRECT_MS);
    return () => clearTimeout(timer);
  }, [doneId, router, app.id]);

  if (profile.isPending) return <ProfileSkeleton />;
  if (profile.isError) {
    const error = profile.error;
    if (error instanceof ApiError && error.code === "profile_not_found") {
      return <NotFound appId={app.id} />;
    }
    return (
      <Card className="px-6">
        <ErrorState error={error} onRetry={() => void profile.refetch()} />
      </Card>
    );
  }

  const pages = profile.data.pages.flatMap((p) => (p.status === 200 ? [p.data] : []));
  const first = pages[0];
  if (!first) return <NotFound appId={app.id} />;
  const sessions = pages.flatMap((p) => p.sessions);

  return (
    <div className="flex flex-col gap-6">
      <Link
        href={searchHref(app.id)}
        className="inline-flex w-fit items-center gap-1 text-body-md text-fg-link hover:underline"
      >
        <ArrowLeft aria-hidden="true" className="size-4" />К поиску
      </Link>
      <ProfileCard
        profile={first}
        timezone={app.timezone}
        deletion={deletion}
        onDelete={() => setDialogOpen(true)}
      />
      <Card className="gap-4 px-6">
        <h3 className="text-heading-3">Устройства</h3>
        <InstallsTable installs={first.installs} timezone={app.timezone} />
      </Card>
      <Card className="gap-4 px-6">
        <h3 className="text-heading-3">Сессии</h3>
        <SessionsTable
          sessions={sessions}
          timezone={app.timezone}
          hasMore={profile.hasNextPage}
          loadingMore={profile.isFetchingNextPage}
          onLoadMore={() => void profile.fetchNextPage()}
        />
      </Card>
      <ProfileDeleteDialog
        appId={app.id}
        appName={app.name}
        personKey={personKey}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onCreated={setCreated}
      />
    </div>
  );
}
