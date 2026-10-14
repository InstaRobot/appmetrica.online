import { Badge } from "@/components/ui/badge";
import type { DeletionStatus } from "@/lib/api/gen/model";
import { cn } from "@/lib/utils";

type DeletionStatusBadgeProps = { status: DeletionStatus; className?: string };

type Tone = "neutral" | "info" | "success" | "danger";

// Статус запроса на удаление (14-ux-ui-spec.md §16.11). pending — «В очереди» по AC-042.1 (OQ-058).
const STATUS: Record<DeletionStatus, { label: string; tone: Tone }> = {
  pending: { label: "В очереди", tone: "neutral" },
  running: { label: "Выполняется", tone: "info" },
  done: { label: "Удалено", tone: "success" },
  failed: { label: "Ошибка", tone: "danger" },
};

const TONE_CLASS: Record<Tone, string> = {
  neutral: "bg-secondary text-secondary-foreground",
  info: "bg-selected text-fg-link",
  success: "bg-success-50 text-positive",
  danger: "bg-destructive/10 text-destructive",
};

export function DeletionStatusBadge({ status, className }: DeletionStatusBadgeProps) {
  const { label, tone } = STATUS[status];
  return (
    <Badge data-tone={tone} className={cn(TONE_CLASS[tone], className)}>
      {label}
    </Badge>
  );
}
