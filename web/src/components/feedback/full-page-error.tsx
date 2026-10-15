"use client";

import { Copy, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { copyText } from "@/lib/clipboard";
import { fmt } from "@/lib/i18n/format-message";
import type { Messages } from "@/messages";

export type ErrorTexts = Messages["system"]["error"];

// «Код запроса» с кнопкой копирования: request_id для поиска в логах.
export function RequestId({ id, texts }: { id: string; texts: ErrorTexts }) {
  return (
    <p className="flex items-center gap-1 text-caption text-fg-muted">
      <span>{fmt(texts.requestId, { id })}</span>
      <Button
        variant="ghost"
        size="icon-xs"
        aria-label={texts.copyRequestId}
        onClick={() => void copyText(id, { copied: texts.copied, failed: texts.copyFailed })}
      >
        <Copy aria-hidden="true" />
      </Button>
    </p>
  );
}

type FullPageErrorProps = {
  texts: ErrorTexts;
  requestId?: string | undefined;
  onRetry?: (() => void) | undefined;
};

// Полностраничная ошибка (14-ux-ui-spec.md S-14b): заголовок в фокусе, повтор и путь в портфель. Тексты
// приходят пропсом, поэтому компонент работает и вне MessagesProvider — в корневой границе app/error.tsx.
export function FullPageError({ texts, requestId, onRetry }: FullPageErrorProps) {
  const heading = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    heading.current?.focus();
  }, []);

  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center gap-4 px-8 text-center">
      <TriangleAlert aria-hidden="true" className="size-12 text-negative" />
      <h1 ref={heading} tabIndex={-1} className="text-heading-1 outline-none">
        {texts.title}
      </h1>
      <p className="text-body-md text-muted-foreground">{texts.text}</p>
      <div className="flex gap-2">
        {onRetry ? <Button onClick={onRetry}>{texts.retry}</Button> : null}
        <Button variant="outline" asChild>
          <Link href="/portfolio">{texts.toPortfolio}</Link>
        </Button>
      </div>
      {requestId ? <RequestId id={requestId} texts={texts} /> : null}
    </div>
  );
}
