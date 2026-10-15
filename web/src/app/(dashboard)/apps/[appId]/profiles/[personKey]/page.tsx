import type { Metadata } from "next";
import { ProfilePanel } from "@/components/profiles/profile-panel";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";

export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).profiles.profileTitle };
}

// S-09 `/apps/[appId]/profiles/[personKey]`: карточка человека и удаление данных (FR-016, FR-019).
// personKey — сегмент `u%3A…`/`i%3A…`, его разбирает lib/person-key.ts.
export default async function ProfilePage({
  params,
}: PageProps<"/apps/[appId]/profiles/[personKey]">) {
  const { appId, personKey } = await params;
  return <ProfilePanel appId={appId} personKey={personKey} />;
}
