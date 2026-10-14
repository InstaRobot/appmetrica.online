"use client";

import { ErrorState } from "@/components/feedback/error-state";

type Props = { error: Error & { digest?: string }; reset: () => void };

// S-14b: граница ошибок сегмента. Текст ошибки не показывается — только request_id для поиска в логах.
export default function ErrorPage({ error, reset }: Props) {
  return (
    <main className="flex min-h-screen items-center justify-center">
      <ErrorState error={error} onRetry={reset} variant="full-page" />
    </main>
  );
}
