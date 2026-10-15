"use client";

import type { ReactNode } from "react";
import { useMessages } from "@/lib/i18n/client";

// Контейнер скелета: role="status" с подписью «Загрузка…» на языке интерфейса. Клиентский только он —
// разметка скелетов остаётся серверной (loading.tsx и fallback страниц), в JS попадает одна обёртка.
export function LoadingStatus({
  className,
  children,
}: {
  className?: string | undefined;
  children: ReactNode;
}) {
  return (
    <div role="status" aria-label={useMessages().common.loading} className={className}>
      {children}
    </div>
  );
}
