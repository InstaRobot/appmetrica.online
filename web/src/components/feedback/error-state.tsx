"use client";

import { CircleAlert, Copy, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef } from "react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { copyText } from "@/lib/clipboard";
import { toUiError } from "@/lib/errors";

type ApiErrorLike = { code: string; message: string; request_id?: string; status?: number };
type ErrorStateProps = {
  error: ApiErrorLike | Error;
  onRetry?: () => void;
  correlationId?: string;
  variant?: "inline" | "full-page";
};

function RequestId({ id }: { id: string }) {
  return (
    <p className="flex items-center gap-1 text-caption text-fg-muted">
      <span>Код запроса: {id}</span>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label="Копировать код запроса"
        onClick={() => void copyText(id)}
      >
        <Copy aria-hidden="true" />
      </Button>
    </p>
  );
}

// Ошибка загрузки (14-ux-ui-spec.md §8.3, §16.3). Сырой текст ошибки не показывается — только текст
// из lib/errors.ts и request_id для поиска в логах.
export function ErrorState({ error, onRetry, correlationId, variant = "inline" }: ErrorStateProps) {
  const ui = toUiError(error);
  const requestId = correlationId ?? ui.requestId;

  if (variant === "full-page") {
    return <FullPageError requestId={requestId} onRetry={onRetry} />;
  }

  return (
    <Alert variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertDescription className="flex flex-col items-start gap-2 text-negative">
        <p className="text-body-md">{ui.message}</p>
        {onRetry ? (
          <Button variant="outline" size="sm" onClick={onRetry}>
            Повторить
          </Button>
        ) : null}
        {requestId ? <RequestId id={requestId} /> : null}
      </AlertDescription>
    </Alert>
  );
}

function FullPageError({
  requestId,
  onRetry,
}: {
  requestId?: string | undefined;
  onRetry?: (() => void) | undefined;
}) {
  const heading = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    heading.current?.focus();
  }, []);

  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 px-8 text-center">
      <TriangleAlert aria-hidden="true" className="size-12 text-negative" />
      <h1 ref={heading} tabIndex={-1} className="text-heading-1 outline-none">
        Что-то пошло не так
      </h1>
      <p className="text-body-md text-muted-foreground">
        Произошла непредвиденная ошибка. Попробуйте обновить страницу.
      </p>
      <div className="flex gap-2">
        {onRetry ? <Button onClick={onRetry}>Попробовать снова</Button> : null}
        <Button variant="outline" asChild>
          <Link href="/portfolio">В портфель</Link>
        </Button>
      </div>
      {requestId ? <RequestId id={requestId} /> : null}
    </div>
  );
}
