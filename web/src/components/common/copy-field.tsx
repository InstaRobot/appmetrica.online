"use client";

import { Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useCopyText } from "@/lib/clipboard";
import { useMessages } from "@/lib/i18n/client";
import { fmt } from "@/lib/i18n/format-message";
import { cn } from "@/lib/utils";

type CopyFieldProps = {
  value: string;
  /** Для aria-label поля и кнопки: «Копировать {label}» (common.copy.value). */
  label: string;
  /** Моноширинный шрифт, по умолчанию true. */
  mono?: boolean;
  /** Показать первые 11 символов + «••••»; копируется полное значение. */
  masked?: boolean;
};

// CopyField (14-ux-ui-spec.md §16.13): ключи, ID, ссылки и команды в поле только для чтения.
export function CopyField({ value, label, mono = true, masked = false }: CopyFieldProps) {
  const m = useMessages().common.copy;
  const copy = useCopyText();
  return (
    <div className="flex items-center gap-2">
      <Input
        readOnly
        value={masked ? `${value.slice(0, 11)}••••` : value}
        aria-label={label}
        onFocus={(e) => e.currentTarget.select()}
        // Телефон: поле и кнопка — цели нажатия 44 px, с md — прежние размеры.
        className={cn("h-11 bg-subtle shadow-inner md:h-8", mono && "font-mono text-code")}
      />
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="size-11 md:size-9"
        aria-label={fmt(m.value, { label })}
        onClick={() => void copy(value)}
      >
        <Copy aria-hidden="true" />
      </Button>
    </div>
  );
}
