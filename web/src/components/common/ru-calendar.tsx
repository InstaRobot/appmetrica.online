"use client";

import type { ComponentProps } from "react";
import { ru } from "react-day-picker/locale";
import { Calendar } from "@/components/ui/calendar";

// Календарь shadcn с русской локалью react-day-picker. Подключается только через LazyCalendar.
export function RuCalendar(props: ComponentProps<typeof Calendar>) {
  return <Calendar locale={ru} {...props} />;
}
