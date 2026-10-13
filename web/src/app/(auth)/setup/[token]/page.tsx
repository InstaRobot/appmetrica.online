import type { Metadata } from "next";
import { headers } from "next/headers";
import { Suspense } from "react";
import { SetupPasskeyForm } from "@/components/auth/setup-passkey-form";
import { Skeleton } from "@/components/ui/skeleton";
import { deviceNameFromUserAgent } from "@/lib/webauthn";

export const metadata: Metadata = { title: "Регистрация passkey" };

// S-02: регистрация passkey по одноразовой ссылке из `appmetrica admin passkey-link` (TTL 15 минут).
export default function SetupPage({ params }: PageProps<"/setup/[token]">) {
  return (
    <>
      <h1 id="auth-title" className="text-heading-1">
        Регистрация passkey
      </h1>
      <p className="text-body-md text-muted-foreground">Ссылка одноразовая и действует 15 минут.</p>
      <Suspense fallback={<Skeleton className="h-24 w-full" />}>
        <SetupBody params={params} />
      </Suspense>
    </>
  );
}

async function SetupBody({ params }: Pick<PageProps<"/setup/[token]">, "params">) {
  const [{ token }, h] = await Promise.all([params, headers()]);
  return (
    <SetupPasskeyForm
      token={token}
      defaultName={deviceNameFromUserAgent(h.get("user-agent") ?? "")}
    />
  );
}
