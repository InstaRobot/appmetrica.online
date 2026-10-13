"use client";

import { TriangleAlert } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/lib/api/fetcher";

// Страница портфеля S-03 появляется в T-037; до этого typedRoutes не знает маршрут "/".
const PORTFOLIO = "/" as Route;

type Props = { error: Error & { digest?: string }; reset: () => void };

// S-14b: граница ошибок сегмента. Текст ошибки не показывается — только request_id для поиска в логах.
export default function ErrorPage({ error, reset }: Props) {
  const heading = useRef<HTMLHeadingElement>(null);
  const requestId = error instanceof ApiError ? error.requestId : "";

  useEffect(() => {
    heading.current?.focus();
  }, []);

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-4 px-8 text-center">
      <TriangleAlert aria-hidden="true" className="size-12 text-negative" />
      <h1 ref={heading} tabIndex={-1} className="text-heading-1 outline-none">
        Что-то пошло не так
      </h1>
      <p className="text-body-md text-muted-foreground">
        Произошла непредвиденная ошибка. Попробуйте обновить страницу.
      </p>
      <div className="flex gap-2">
        <Button onClick={reset}>Попробовать снова</Button>
        <Button variant="outline" asChild>
          <Link href={PORTFOLIO}>В портфель</Link>
        </Button>
      </div>
      {requestId ? <p className="text-caption text-fg-subtle">Код запроса: {requestId}</p> : null}
    </main>
  );
}
