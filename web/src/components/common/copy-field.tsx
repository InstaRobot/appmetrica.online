"use client";

import { Copy } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { copyText } from "@/lib/clipboard";
import { cn } from "@/lib/utils";

type CopyFieldProps = {
  value: string;
  /** Для aria-label поля и кнопки: «Копировать {label}». */
  label: string;
  /** Моноширинный шрифт, по умолчанию true. */
  mono?: boolean;
  /** Показать первые 11 символов + «••••»; копируется полное значение. */
  masked?: boolean;
};

// CopyField (14-ux-ui-spec.md §16.13): ключи, ID, ссылки и команды в поле только для чтения.
export function CopyField({ value, label, mono = true, masked = false }: CopyFieldProps) {
  return (
    <div className="flex items-center gap-2">
      <Input
        readOnly
        value={masked ? `${value.slice(0, 11)}••••` : value}
        aria-label={label}
        onFocus={(e) => e.currentTarget.select()}
        className={cn("bg-subtle shadow-inner", mono && "font-mono text-code")}
      />
      <Button
        type="button"
        variant="outline"
        size="icon"
        aria-label={`Копировать ${label}`}
        onClick={() => void copyText(value)}
      >
        <Copy aria-hidden="true" />
      </Button>
    </div>
  );
}
