import type { Locale } from "@/lib/i18n/locale";
import { en } from "./en";
import { ru } from "./ru";

// Словари интерфейса. Импортируют их только серверные модули и тесты: клиентские компоненты получают
// словарь пропсами MessagesProvider (RSC-данные), иначе оба языка попали бы в initial JS (проверка
// pnpm check:bundle).
export type Messages = typeof en;

const MESSAGES: Record<Locale, Messages> = { en, ru };

export function getMessages(locale: Locale): Messages {
  return MESSAGES[locale];
}
