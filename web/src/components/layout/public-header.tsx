import type { Route } from "next";
import Link from "next/link";
import { cn } from "@/lib/utils";
import { Logo } from "./logo";
import { ThemeToggle } from "./theme-toggle";

// Шапка публичных страниц: лендинг S-00 и документация для гостя. Без запросов к API — шапка
// дашборда грузит список приложений, и 401 увёл бы гостя на вход.
export function PublicHeader({ className }: { className?: string }) {
  return (
    <header className={cn("flex h-16 w-full items-center gap-4 px-8", className)}>
      <Link href="/" aria-label="appmetrica.online — на главную" className="rounded-md">
        <Logo />
      </Link>
      <div className="ml-auto flex items-center gap-2">
        <Link
          href={"/docs" as Route}
          className="inline-flex h-9 items-center rounded-lg px-3 text-sm font-semibold text-fg-muted transition-colors outline-none hover:bg-subtle hover:text-foreground focus-visible:ring-3 focus-visible:ring-ring/50"
        >
          Документация
        </Link>
        <ThemeToggle className="border border-(--border-strong) bg-card" />
        <Link
          href="/login"
          className="inline-flex h-9 items-center rounded-lg bg-linear-to-b from-(--primary-top) to-primary px-4 text-sm font-semibold text-primary-foreground shadow-(--shadow-primary) transition-colors outline-none hover:from-(--primary-hover) hover:to-(--primary-hover) focus-visible:ring-3 focus-visible:ring-ring/50"
        >
          Войти
        </Link>
      </div>
    </header>
  );
}
