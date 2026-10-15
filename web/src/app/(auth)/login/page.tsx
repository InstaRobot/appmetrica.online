import type { Metadata, Route } from "next";
import { redirect } from "next/navigation";
import { Suspense } from "react";
import { PasskeyButton } from "@/components/auth/passkey-button";
import { PASSKEY_LINK_COMMAND, PasskeyLinkCommand } from "@/components/auth/passkey-link-command";
import { richText } from "@/components/common/rich-text";
import { Skeleton } from "@/components/ui/skeleton";
import { getAuthState } from "@/lib/api/gen/endpoints/admin-auth/admin-auth";
import { getRequestLocale } from "@/lib/i18n/server";
import { safeNext } from "@/lib/safe-next";
import { getMessages, type Messages } from "@/messages";

// Язык — из cookie и Accept-Language: страница и так ждёт запроса (состояние auth, layout входа).
export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).auth.login.metaTitle };
}

// S-01: вход по passkey. Состояние auth читается на сервере до рендера: вошедшего владельца сразу
// отправляем на next, без passkey — показываем инструкцию по passkey-link. Layout входа держит страницу
// под Suspense, поэтому язык запроса читается прямо здесь.
export default async function LoginPage({ searchParams }: PageProps<"/login">) {
  const m = getMessages(await getRequestLocale()).auth.login;
  return (
    <>
      <h1 id="auth-title" className="text-heading-1">
        {m.title}
      </h1>
      <Suspense fallback={<Skeleton className="h-10 w-full" />}>
        <LoginBody searchParams={searchParams} m={m} />
      </Suspense>
    </>
  );
}

async function LoginBody({
  searchParams,
  m,
}: Pick<PageProps<"/login">, "searchParams"> & { m: Messages["auth"]["login"] }) {
  const [{ next }, state] = await Promise.all([searchParams, getAuthState()]);
  const target = safeNext(next);
  if (state.data.authenticated) redirect(target as Route);

  if (state.data.setup_required) {
    return (
      <div className="flex flex-col gap-3 rounded-lg border border-border bg-subtle p-4">
        <p className="text-body-md">
          {richText(m.setupRequired, {
            command: <code className="text-code">{PASSKEY_LINK_COMMAND}</code>,
          })}
        </p>
        <PasskeyLinkCommand />
      </div>
    );
  }

  return (
    <>
      <p className="text-body-md text-muted-foreground">{m.hint}</p>
      <PasskeyButton next={target} />
    </>
  );
}
