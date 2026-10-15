"use client";

import { Moon, Sun } from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { useTheme } from "@/hooks/use-theme";
import { useMessages } from "@/lib/i18n/client";
import { applyTheme, type Theme } from "@/lib/theme";
import { cn } from "@/lib/utils";

// Переключатель светлой/тёмной темы (шапка дашборда и лендинг). Выбор сохраняется в localStorage.
export function ThemeToggle({ className }: { className?: string }) {
  const m = useMessages().layout.theme;
  const theme = useTheme();
  const next: Theme = theme === "dark" ? "light" : "dark";
  const Icon = theme === "dark" ? Sun : Moon;

  return (
    <button
      type="button"
      aria-label={next === "dark" ? m.dark : m.light}
      onClick={() => applyTheme(next)}
      className={cn(
        buttonVariants({ variant: "ghost", size: "icon-lg" }),
        "text-fg-muted",
        className,
      )}
    >
      <Icon aria-hidden="true" className="size-5" />
    </button>
  );
}
