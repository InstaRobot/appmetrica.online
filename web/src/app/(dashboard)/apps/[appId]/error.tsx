"use client";

import { ErrorState } from "@/components/feedback/error-state";

type AppErrorProps = { error: Error & { digest?: string }; reset: () => void };

// Непредвиденная ошибка рендера вкладки приложения: шапка и вкладки остаются, «Повторить» перерисовывает отчёт.
export default function AppError({ error, reset }: AppErrorProps) {
  return (
    <ErrorState
      error={error}
      onRetry={reset}
      {...(error.digest ? { correlationId: error.digest } : {})}
    />
  );
}
