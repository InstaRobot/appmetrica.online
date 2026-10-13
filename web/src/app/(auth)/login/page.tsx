import type { Metadata, Route } from "next";
import { redirect } from "next/navigation";
import { Suspense } from "react";
import { PasskeyButton } from "@/components/auth/passkey-button";
import { PasskeyLinkCommand } from "@/components/auth/passkey-link-command";
import { Skeleton } from "@/components/ui/skeleton";
import { getAuthState } from "@/lib/api/gen/endpoints/admin-auth/admin-auth";
import { safeNext } from "@/lib/safe-next";

export const metadata: Metadata = { title: "Вход" };

// S-01: вход по passkey. Состояние auth читается на сервере до рендера: вошедшего владельца сразу
// отправляем на next, без passkey — показываем инструкцию по passkey-link.
export default function LoginPage({ searchParams }: PageProps<"/login">) {
  return (
    <>
      <h1 id="auth-title" className="text-heading-1">
        Вход в appmetrica.online
      </h1>
      <Suspense fallback={<Skeleton className="h-10 w-full" />}>
        <LoginBody searchParams={searchParams} />
      </Suspense>
    </>
  );
}

async function LoginBody({ searchParams }: Pick<PageProps<"/login">, "searchParams">) {
  const [{ next }, state] = await Promise.all([searchParams, getAuthState()]);
  const target = safeNext(next);
  if (state.data.authenticated) redirect(target as Route);

  if (state.data.setup_required) {
    return (
      <div className="flex flex-col gap-3 rounded-lg border border-border bg-subtle p-4">
        <p className="text-body-md">
          Passkey ещё не настроен. На сервере выполните{" "}
          <code className="text-code">appmetrica admin passkey-link</code> и откройте выведенную
          ссылку.
        </p>
        <PasskeyLinkCommand />
      </div>
    );
  }

  return (
    <>
      <p className="text-body-md text-muted-foreground">
        Используйте passkey на этом устройстве или телефоне.
      </p>
      <PasskeyButton next={target} />
    </>
  );
}
