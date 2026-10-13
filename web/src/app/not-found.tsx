import { FileQuestion } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { Button } from "@/components/ui/button";

// Страница портфеля S-03 появляется в T-037; до этого typedRoutes не знает маршрут "/".
const PORTFOLIO = "/" as Route;

// S-14a: глобальная 404 (14-ux-ui-spec.md §5, Screen S-14).
export default function NotFound() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-4 px-8 text-center">
      <FileQuestion aria-hidden="true" className="size-12 text-fg-subtle" />
      <h1 className="text-heading-1">Страница не найдена</h1>
      <p className="text-body-md text-muted-foreground">
        Проверьте адрес или вернитесь в портфель.
      </p>
      <Button asChild>
        <Link href={PORTFOLIO}>В портфель</Link>
      </Button>
    </main>
  );
}
