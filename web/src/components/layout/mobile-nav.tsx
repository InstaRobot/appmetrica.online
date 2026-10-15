"use client";

import { Menu } from "lucide-react";
import type { Route } from "next";
import { lazy, Suspense, useState } from "react";
import { Button } from "@/components/ui/button";
import { useMessages } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";

export type MobileNavItem = { href: Route; label: string; active?: boolean };

export type MobileNavProps = {
  /** panel — шапка дашборда (есть выход), public — лендинг и документация для гостя. */
  variant: "panel" | "public";
  items: MobileNavItem[];
  demo?: boolean;
};

// Панель грузится отдельным чанком при первом нажатии: на десктопе её код не нужен вовсе (бюджет initial JS).
const MobileNavPanel = lazy(() =>
  import("./mobile-nav-panel").then((m) => ({ default: m.MobileNavPanel })),
);

// Кнопка ☰ шапки на узком экране: разделы, язык, тема и выход — в боковой панели. С какой ширины её
// прятать, решает шапка (md:hidden у публичной, lg:hidden у дашборда).
export function MobileNav({ className, ...props }: MobileNavProps & { className?: string }) {
  const m = useMessages().layout.mobileNav;
  const [open, setOpen] = useState(false);
  const [loaded, setLoaded] = useState(false);

  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="icon-lg"
        aria-label={m.open}
        aria-haspopup="dialog"
        aria-expanded={open}
        className={cn("size-11", className)}
        onClick={() => {
          setLoaded(true);
          setOpen(true);
        }}
      >
        <Menu aria-hidden="true" className="size-5" />
      </Button>
      {loaded ? (
        <Suspense fallback={null}>
          <MobileNavPanel {...props} open={open} onOpenChange={setOpen} />
        </Suspense>
      ) : null}
    </>
  );
}
