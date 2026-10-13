"use client";

import { CircleAlert, KeyRound, Loader2 } from "lucide-react";
import type { Route } from "next";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { safeNext } from "@/lib/safe-next";
import {
  cancelPasskeyCeremony,
  loginWithPasskey,
  passkeyErrorMessage,
  supportsPasskeyAutofill,
  supportsPasskeys,
} from "@/lib/webauthn";

const PASSKEY_HELP_URL = "https://passkeys.dev/device-support/";

const subscribeNever = () => () => {};

// Поддержка WebAuthn известна только в браузере; на сервере считаем, что она есть, чтобы не мигать
// предупреждением у большинства. После гидрации React перерисует компонент с настоящим значением.
export function usePasskeySupport(): boolean {
  return useSyncExternalStore(subscribeNever, supportsPasskeys, () => true);
}

export function PasskeyUnsupported() {
  return (
    <p className="text-body-sm text-muted-foreground">
      <span>Браузер не поддерживает passkey</span>.{" "}
      <a
        href={PASSKEY_HELP_URL}
        target="_blank"
        rel="noreferrer noopener"
        className="text-fg-link underline"
      >
        Где работают passkey
      </a>
    </p>
  );
}

type PasskeyButtonProps = {
  next?: string;
  onError?: (message: string) => void;
};

// S-01: вход по passkey кнопкой и через автозаполнение (conditional UI).
export function PasskeyButton({ next, onError }: PasskeyButtonProps) {
  const router = useRouter();
  const supported = usePasskeySupport();
  const target = safeNext(next) as Route;
  const [waiting, setWaiting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const conditional = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!supported) return;
    const controller = new AbortController();
    conditional.current = controller;
    void (async () => {
      try {
        if (!(await supportsPasskeyAutofill()) || controller.signal.aborted) return;
        await loginWithPasskey({ autofill: true, signal: controller.signal });
        router.replace(target);
      } catch {
        // Автозаполнение молчит: его отменяет клик по кнопке или уход со страницы, а сбой не мешает
        // войти кнопкой.
      }
    })();
    return () => {
      controller.abort();
      cancelPasskeyCeremony();
    };
  }, [supported, router, target]);

  useEffect(() => {
    if (error && !waiting) buttonRef.current?.focus();
  }, [error, waiting]);

  async function login() {
    conditional.current?.abort();
    setWaiting(true);
    setError(null);
    try {
      await loginWithPasskey({ autofill: false });
      router.replace(target);
    } catch (err) {
      const message = passkeyErrorMessage(err, "login");
      setError(message);
      setWaiting(false);
      onError?.(message);
    }
  }

  return (
    <div className="flex flex-col gap-3">
      {supported && (
        <input
          type="text"
          name="username"
          autoComplete="username webauthn"
          tabIndex={-1}
          aria-hidden="true"
          className="sr-only"
        />
      )}
      <Button
        ref={buttonRef}
        type="button"
        className="h-10 w-full"
        disabled={!supported || waiting}
        onClick={login}
      >
        {waiting ? (
          <>
            <Loader2 aria-hidden="true" className="animate-spin" />
            Ожидаем подтверждение…
          </>
        ) : (
          <>
            <KeyRound aria-hidden="true" />
            Войти с passkey
          </>
        )}
      </Button>
      {!supported && <PasskeyUnsupported />}
      {error && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}
