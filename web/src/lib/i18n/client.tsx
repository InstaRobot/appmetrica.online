"use client";

import { createContext, type ReactNode, useContext } from "react";
import { createFormatters, type Formatters } from "@/lib/format";
import type { Messages } from "@/messages";
import type { Locale } from "./locale";

type I18n = { locale: Locale; messages: Messages };

const I18nContext = createContext<I18n | null>(null);

// Словарь приходит из серверного layout пропсом (RSC-данные), поэтому в клиентский бандл не попадает.
export function MessagesProvider({ locale, messages, children }: I18n & { children: ReactNode }) {
  return <I18nContext.Provider value={{ locale, messages }}>{children}</I18nContext.Provider>;
}

function useI18n(): I18n {
  const value = useContext(I18nContext);
  if (!value) throw new Error("useMessages: компонент вне MessagesProvider");
  return value;
}

export function useMessages(): Messages {
  return useI18n().messages;
}

export function useLocale(): Locale {
  return useI18n().locale;
}

// Форматтеры языка: createFormatters кэширует набор на язык, повторный вызов дешёвый.
export function useFormat(): Formatters {
  return createFormatters(useI18n().locale);
}
