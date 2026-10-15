"use client";

import { CircleAlert, CircleCheck, Loader2, TimerOff } from "lucide-react";
import type { Route } from "next";
import { useRouter } from "next/navigation";
import { type FormEvent, useEffect, useId, useRef, useState } from "react";
import { richText } from "@/components/common/rich-text";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError } from "@/lib/api/fetcher";
import { useMessages } from "@/lib/i18n/client";
import {
  isSetupLinkExpired,
  passkeyErrorMessage,
  registerPasskeyBySetupLink,
  validateDeviceName,
} from "@/lib/webauthn";
import { PasskeyUnsupported, usePasskeySupport } from "./passkey-button";
import { PASSKEY_LINK_COMMAND, PasskeyLinkCommand } from "./passkey-link-command";

// Страница портфеля S-03 появляется в T-037; до этого typedRoutes не знает маршрут "/".
const PORTFOLIO = "/portfolio" as Route;
const REDIRECT_DELAY_MS = 800;

type Status = "idle" | "submitting" | "success" | "expired" | "error";

type SetupPasskeyFormProps = { token: string; defaultName: string };

// S-02: регистрация passkey по одноразовой ссылке. Успех ставит cookie сессии и ведёт в портфель.
export function SetupPasskeyForm({ token, defaultName }: SetupPasskeyFormProps) {
  const m = useMessages().auth;
  const router = useRouter();
  const supported = usePasskeySupport();
  const [name, setName] = useState(defaultName);
  const [nameError, setNameError] = useState<string | null>(null);
  const [status, setStatus] = useState<Status>("idle");
  const [error, setError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const inputId = useId();
  const nameErrorId = useId();

  useEffect(() => {
    if (status !== "success") return;
    const timer = setTimeout(() => router.replace(PORTFOLIO), REDIRECT_DELAY_MS);
    return () => clearTimeout(timer);
  }, [status, router]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const invalid = validateDeviceName(name, m.deviceName);
    setNameError(invalid);
    if (invalid) {
      inputRef.current?.focus();
      return;
    }
    setStatus("submitting");
    try {
      await registerPasskeyBySetupLink(token, name.trim());
      setStatus("success");
    } catch (err) {
      if (isSetupLinkExpired(err)) {
        setStatus("expired");
      } else if (err instanceof ApiError && err.code === "invalid_request" && hasNameField(err)) {
        setNameError(m.deviceName.required);
        setStatus("idle");
        inputRef.current?.focus();
      } else {
        setError(passkeyErrorMessage(err, "setup", m.passkeyErrors));
        setStatus("error");
      }
    }
  }

  if (status === "expired") {
    return (
      <div role="status" className="flex flex-col items-center gap-3 text-center">
        <TimerOff aria-hidden="true" className="size-10 text-fg-subtle" />
        <h2 className="text-heading-3">{m.setup.expiredTitle}</h2>
        <p className="text-body-md text-muted-foreground">
          {richText(m.setup.expiredHint, {
            command: <code className="text-code">{PASSKEY_LINK_COMMAND}</code>,
          })}
        </p>
        <PasskeyLinkCommand />
      </div>
    );
  }

  const busy = status === "submitting" || status === "success";
  return (
    <form noValidate onSubmit={submit} className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <Label htmlFor={inputId}>
          {m.setup.deviceName} <span aria-hidden="true">*</span>
        </Label>
        <Input
          ref={inputRef}
          id={inputId}
          name="name"
          required
          autoComplete="off"
          // Телефон: поле 44 px и шрифт 16 px — iOS не увеличивает страницу при фокусе.
          className="h-11 text-lg md:h-8 md:text-sm"
          value={name}
          onChange={(e) => setName(e.target.value)}
          disabled={busy}
          aria-invalid={nameError ? true : undefined}
          aria-describedby={nameError ? nameErrorId : undefined}
        />
        {nameError && (
          <p id={nameErrorId} className="text-body-sm text-destructive">
            {nameError}
          </p>
        )}
      </div>
      <Button type="submit" className="h-11 w-full md:h-10" disabled={!supported || busy}>
        {status === "submitting" ? (
          <>
            <Loader2 aria-hidden="true" className="animate-spin" />
            {m.setup.submitting}
          </>
        ) : (
          m.setup.submit
        )}
      </Button>
      {!supported && <PasskeyUnsupported />}
      {status === "success" && (
        <div
          role="status"
          className="flex items-center gap-2 rounded-lg border border-(--color-success-500) bg-(--color-success-50) px-3 py-2 text-body-sm text-positive"
        >
          <CircleCheck aria-hidden="true" className="size-4" />
          {m.setup.success}
        </div>
      )}
      {status === "error" && (
        <Alert variant="destructive">
          <CircleAlert aria-hidden="true" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
    </form>
  );
}

function hasNameField(err: ApiError): boolean {
  const fields = err.details?.fields;
  return typeof fields === "object" && fields !== null && "name" in fields;
}
