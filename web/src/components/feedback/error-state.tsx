"use client";

import { CircleAlert } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { useMessages } from "@/lib/i18n/client";
import { useUiError } from "@/lib/use-ui-error";
import { FullPageError, RequestId } from "./full-page-error";

type ApiErrorLike = { code: string; message: string; request_id?: string; status?: number };
type ErrorStateProps = {
  error: ApiErrorLike | Error;
  onRetry?: () => void;
  correlationId?: string;
  variant?: "inline" | "full-page";
};

// Ошибка загрузки (14-ux-ui-spec.md §8.3, §16.3). Сырой текст ошибки не показывается — только текст
// из lib/errors.ts и request_id для поиска в логах.
export function ErrorState({ error, onRetry, correlationId, variant = "inline" }: ErrorStateProps) {
  const m = useMessages();
  const uiError = useUiError();
  const ui = uiError(error);
  const requestId = correlationId ?? ui.requestId;

  if (variant === "full-page") {
    return <FullPageError texts={m.system.error} requestId={requestId} onRetry={onRetry} />;
  }

  return (
    <Alert variant="destructive">
      <CircleAlert aria-hidden="true" />
      <AlertDescription className="flex flex-col items-start gap-2 text-negative">
        <p className="text-body-md">{ui.message}</p>
        {onRetry ? (
          <Button variant="outline" size="sm" onClick={onRetry}>
            {m.common.retry}
          </Button>
        ) : null}
        {requestId ? <RequestId id={requestId} texts={m.system.error} /> : null}
      </AlertDescription>
    </Alert>
  );
}
