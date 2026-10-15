import type { Metadata } from "next";
import { headers } from "next/headers";
import { Suspense } from "react";
import { SetupPasskeyForm } from "@/components/auth/setup-passkey-form";
import { Skeleton } from "@/components/ui/skeleton";
import { getRequestLocale } from "@/lib/i18n/server";
import { deviceNameFromUserAgent } from "@/lib/webauthn";
import { getMessages, type Messages } from "@/messages";

// Язык — из cookie и Accept-Language: страница и так ждёт запроса (токен, User-Agent, layout входа).
export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).auth.setup.metaTitle };
}

// S-02: регистрация passkey по одноразовой ссылке из `appmetrica admin passkey-link` (TTL 15 минут).
// Layout входа держит страницу под Suspense, поэтому язык запроса читается прямо здесь.
export default async function SetupPage({ params }: PageProps<"/setup/[token]">) {
  const m = getMessages(await getRequestLocale()).auth.setup;
  return (
    <>
      <h1 id="auth-title" className="text-heading-1">
        {m.title}
      </h1>
      <p className="text-body-md text-muted-foreground">{m.oneTime}</p>
      <Suspense fallback={<Skeleton className="h-24 w-full" />}>
        <SetupBody params={params} m={m} />
      </Suspense>
    </>
  );
}

async function SetupBody({
  params,
  m,
}: Pick<PageProps<"/setup/[token]">, "params"> & { m: Messages["auth"]["setup"] }) {
  const [{ token }, h] = await Promise.all([params, headers()]);
  return (
    <SetupPasskeyForm
      token={token}
      defaultName={deviceNameFromUserAgent(h.get("user-agent") ?? "", m.defaultDeviceName)}
    />
  );
}
