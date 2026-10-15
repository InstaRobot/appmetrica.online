"use client";

import { FullPageError } from "@/components/feedback/full-page-error";
import { useDocumentLocale } from "@/components/feedback/use-document-locale";
import { ApiError } from "@/lib/api/fetcher";
import { systemError as en } from "@/messages/en/system-error";
import { systemError as ru } from "@/messages/ru/system-error";

type Props = { error: Error & { digest?: string }; reset: () => void };

// Граница ошибок стоит над layout групп маршрутов, то есть вне MessagesProvider: язык — из <html lang>,
// тексты — из короткого модуля system-error обоих языков (весь словарь в initial JS не нужен).
const TEXTS = { en, ru };

function requestIdOf(error: unknown): string | undefined {
  if (error instanceof ApiError) return error.requestId || undefined;
  const id = (error as { request_id?: unknown }).request_id;
  return typeof id === "string" && id ? id : undefined;
}

// S-14b: граница ошибок сегмента. Текст ошибки не показывается — только request_id для поиска в логах.
export default function ErrorPage({ error, reset }: Props) {
  return (
    <main className="flex min-h-screen items-center justify-center">
      <FullPageError
        texts={TEXTS[useDocumentLocale()]}
        requestId={requestIdOf(error)}
        onRetry={reset}
      />
    </main>
  );
}
