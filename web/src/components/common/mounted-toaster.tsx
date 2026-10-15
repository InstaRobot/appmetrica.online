"use client";

import { useEffect } from "react";
import { useDocumentLocale } from "@/components/feedback/use-document-locale";
import { Toaster } from "@/components/ui/sonner";
import { markToasterMounted } from "@/lib/toast";
import { systemToaster as en } from "@/messages/en/system-toaster";
import { systemToaster as ru } from "@/messages/ru/system-toaster";

// Toaster, после подписки которого lib/toast начинает показывать тосты: эффект дочернего Sonner
// (подписка на тосты) выполняется раньше эффекта родителя. Он стоит в корневом layout вне
// MessagesProvider, поэтому язык подписей берёт из <html lang>.
export function MountedToaster() {
  useEffect(markToasterMounted, []);
  const t = useDocumentLocale() === "ru" ? ru : en;
  return <Toaster containerAriaLabel={t.region} toastOptions={{ closeButtonAriaLabel: t.close }} />;
}
