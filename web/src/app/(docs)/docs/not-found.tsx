import { FileQuestion } from "lucide-react";
import type { Route } from "next";
import Link from "next/link";
import { Button } from "@/components/ui/button";

// S-13 not-found: неизвестная страница документации, внутри layout доков (сайдбар остаётся).
export default function DocsNotFound() {
  return (
    <main
      id="nd-page"
      className="flex flex-col items-center justify-center gap-4 px-8 py-24 text-center [grid-area:main]"
    >
      <FileQuestion aria-hidden="true" className="size-12 text-fg-subtle" />
      <h1 className="text-heading-1">Страница не найдена</h1>
      <p className="text-body-md text-muted-foreground">Такой страницы в документации нет.</p>
      <Button asChild>
        <Link href={"/docs" as Route}>К быстрому старту</Link>
      </Button>
    </main>
  );
}
