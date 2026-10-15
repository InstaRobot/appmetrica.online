"use client";

import { CircleCheck, Loader2 } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { useEffect, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
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
  const m = useMessages().integration.firstEvent;
  const f = useFormat();
  const previous = useRef(firstEventAt);
  useEffect(() => {
    if (previous.current === null && firstEventAt !== null) {
      toast.success(m.toast);
    }
    previous.current = firstEventAt;
  }, [firstEventAt, m.toast]);

  const received = firstEventAt !== null;

  return (
    <Card
      className={cn(
        "flex-row flex-wrap items-center gap-3 px-4 md:px-6",
        received && "border-positive/40 bg-positive/5",
      )}
    >
      {received ? (
        <CircleCheck aria-hidden="true" className="size-5 shrink-0 text-positive" />
      ) : (
        <Loader2 aria-hidden="true" className="size-5 shrink-0 animate-spin text-fg-muted" />
      )}
      <p role="status" aria-live="polite" aria-label={m.label} className="text-body-md">
        {received ? (
          <>
            <span className="font-medium text-positive">
              {fmt(m.receivedAt, { time: f.formatClock(firstEventAt, timezone) })}
            </span>{" "}
            {fmt(m.connected, { app: appName })}
          </>
        ) : (
          <>
            {m.waiting} <span className="text-fg-muted">{m.polling}</span>
          </>
        )}
      </p>
      {received ? (
        <Button asChild size="sm" className="ml-auto h-11 md:h-8">
          <Link href={audienceHref as Route}>{m.openAudience}</Link>
        </Button>
      ) : null}
    </Card>
  );
}
