"use client";

import type { ComponentProps } from "react";
import { enUS, ru } from "react-day-picker/locale";
import { Calendar } from "@/components/ui/calendar";
import { useLocale } from "@/lib/i18n/client";

// Календарь shadcn на языке интерфейса: локаль react-day-picker (названия месяцев и дней недели, первый
// день недели). Подключается только через LazyCalendar.
export function LocalizedCalendar(props: ComponentProps<typeof Calendar>) {
  return <Calendar locale={useLocale() === "ru" ? ru : enUS} {...props} />;
}
