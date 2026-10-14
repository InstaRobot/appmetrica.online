"use client";

import { Moon, Sun } from "lucide-react";
import { buttonVariants } from "@/components/ui/button";
import { useTheme } from "@/hooks/use-theme";
import { applyTheme, type Theme } from "@/lib/theme";
import { cn } from "@/lib/utils";

// Переключатель светлой/тёмной темы (шапка дашборда и лендинг). Выбор сохраняется в localStorage.
export function ThemeToggle({ className }: { className?: string }) {
  const theme = useTheme();
  const next: Theme = theme === "dark" ? "light" : "dark";
  const Icon = theme === "dark" ? Sun : Moon;

  return (
    <button
      type="button"
      aria-label={next === "dark" ? "Включить тёмную тему" : "Включить светлую тему"}
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
