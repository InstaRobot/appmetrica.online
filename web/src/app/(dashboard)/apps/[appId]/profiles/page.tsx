import type { Metadata } from "next";
import { Suspense } from "react";
import { ProfileSearchPanel } from "@/components/profiles/profile-search-panel";
import { TableSkeleton } from "@/components/skeletons/table-skeleton";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";

export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).profiles.title };
}

// S-08 `/apps/[appId]/profiles`: запрос в `?q=` (nuqs читает useSearchParams), поэтому панель — под Suspense.
export default async function ProfilesPage({ params }: PageProps<"/apps/[appId]/profiles">) {
  const { appId } = await params;
  return (
    <Suspense fallback={<TableSkeleton rows={2} />}>
      <ProfileSearchPanel appId={appId} />
    </Suspense>
  );
}
