import type { Route } from "next";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

type EmptyStateProps = {
  icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  title: string;
  description?: React.ReactNode;
  action?: { label: string; href?: string; onClick?: () => void };
  className?: string;
};

// Пустое состояние (14-ux-ui-spec.md §8.1): иконка 48 px, заголовок, пояснение и одно действие.
export function EmptyState({ icon: Icon, title, description, action, className }: EmptyStateProps) {
  return (
    <div className={cn("flex flex-col items-center gap-3 px-6 py-12 text-center", className)}>
      <Icon aria-hidden className="size-12 text-fg-subtle" />
      <h2 className="text-heading-3">{title}</h2>
      {description ? (
        <p className="max-w-[420px] text-body-md text-muted-foreground">{description}</p>
      ) : null}
      {action?.href ? (
        <Button asChild className="mt-2">
          <Link href={action.href as Route}>{action.label}</Link>
        </Button>
      ) : action ? (
        <Button className="mt-2" onClick={action.onClick}>
          {action.label}
        </Button>
      ) : null}
    </div>
  );
}
