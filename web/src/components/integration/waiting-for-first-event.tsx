"use client";

import { CircleCheck, Loader2 } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { formatClock } from "@/lib/format";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";

type WaitingForFirstEventProps = {
  firstEventAt: string | null;
  appName: string;
  audienceHref: string;
  /** TZ приложения для времени первого события. */
  timezone: string;
};

// WaitingForFirstEvent (14-ux-ui-spec.md S-10): ожидание первого события и переход в «получено».
// Toast — только на переход null → значение в текущей сессии страницы.
export function WaitingForFirstEvent({
  firstEventAt,
  appName,
  audienceHref,
  timezone,
}: WaitingForFirstEventProps) {
  const previous = useRef(firstEventAt);
  useEffect(() => {
    if (previous.current === null && firstEventAt !== null) {
      toast.success("Первое событие получено");
    }
    previous.current = firstEventAt;
  }, [firstEventAt]);

  const received = firstEventAt !== null;

  return (
    <Card
      className={cn(
        "flex-row items-center gap-3 px-6",
        received && "border-positive/40 bg-positive/5",
      )}
    >
      {received ? (
        <CircleCheck aria-hidden="true" className="size-5 shrink-0 text-positive" />
      ) : (
        <Loader2 aria-hidden="true" className="size-5 shrink-0 animate-spin text-fg-muted" />
      )}
      <p role="status" aria-live="polite" aria-label="Первое событие" className="text-body-md">
        {received ? (
          <>
            <span className="font-medium text-positive">
              Первое событие получено {formatClock(firstEventAt, timezone)}
            </span>{" "}
            — {appName} подключено!
          </>
        ) : (
          <>
            Ждём первое событие… <span className="text-fg-muted">Проверяем каждые 3 секунды.</span>
          </>
        )}
      </p>
      {received ? (
        <Button asChild size="sm" className="ml-auto">
          <Link href={audienceHref as Route}>Открыть Аудиторию</Link>
        </Button>
      ) : null}
    </Card>
  );
}
