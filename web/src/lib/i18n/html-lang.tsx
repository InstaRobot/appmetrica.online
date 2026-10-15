"use client";

import { useEffect } from "react";
import type { Locale } from "./locale";

// Точный язык сервера после гидратации: в панели без cookie он берётся из Accept-Language, которого
// inline-скрипт не видит.
export function HtmlLang({ locale }: { locale: Locale }) {
  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);
  return null;
}
