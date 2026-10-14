"use client";

import { useId, useRef, useState } from "react";
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type ConfirmDeleteDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: React.ReactNode;
  /** Строка, которую нужно ввести для подтверждения: название приложения или «УДАЛИТЬ». */
  confirmText: string;
  confirmLabel: string;
  pending?: boolean;
  serverError?: string;
  onConfirm: () => void | Promise<void>;
};

// Подтверждение опасного действия вводом текста (14-ux-ui-spec.md §16.14): фокус на поле, кнопка
// disabled до точного совпадения (после trim) и во время pending; Enter при несовпадении не подтверждает.
// Кнопка подтверждения — не AlertDialogAction: диалог закрывает вызывающий код после успеха.
export function ConfirmDeleteDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmText,
  confirmLabel,
  pending = false,
  serverError,
  onConfirm,
}: ConfirmDeleteDialogProps) {
  const [value, setValue] = useState("");
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open) setValue("");
  }
  const inputRef = useRef<HTMLInputElement>(null);
  const inputId = useId();
  const errorId = useId();
  const matches = value.trim() === confirmText;

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (matches && !pending) void onConfirm();
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent
        className="data-[size=default]:sm:max-w-md"
        onOpenAutoFocus={(e) => {
          e.preventDefault();
          inputRef.current?.focus();
        }}
      >
        <form noValidate onSubmit={submit} className="grid gap-4">
          <AlertDialogHeader>
            <AlertDialogTitle>{title}</AlertDialogTitle>
            <AlertDialogDescription>{description}</AlertDialogDescription>
          </AlertDialogHeader>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={inputId}>Введите {confirmText}</Label>
            <Input
              ref={inputRef}
              id={inputId}
              autoComplete="off"
              spellCheck={false}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              aria-invalid={serverError ? true : undefined}
              aria-describedby={serverError ? errorId : undefined}
            />
            {serverError ? (
              <p id={errorId} className="text-body-sm text-destructive">
                {serverError}
              </p>
            ) : null}
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel type="button">Отмена</AlertDialogCancel>
            <Button type="submit" variant="destructive" disabled={!matches || pending}>
              {pending ? "Удаляем…" : confirmLabel}
            </Button>
          </AlertDialogFooter>
        </form>
      </AlertDialogContent>
    </AlertDialog>
  );
}
