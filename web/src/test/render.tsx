import { QueryClientProvider } from "@tanstack/react-query";
import { type RenderOptions, type RenderResult, render } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";
import type { Locale } from "@/lib/i18n/locale";
import { makeQueryClient } from "@/lib/query-client";
import { renderWithLocale } from "@/test/rtl";

// render с собственным QueryClient на каждый тест: кэш запросов не протекает между тестами,
// повторы выключены, чтобы ошибочные ответы MSW сразу давали состояние ошибки. rerender оставляет тот же
// клиент: кэш между перерисовками сохраняется, как в приложении. Язык по умолчанию — русский (rtl.tsx),
// { locale: "en" } — английский словарь.
export function renderWithClient(
  ui: ReactElement,
  { locale, ...options }: RenderOptions & { locale?: Locale } = {},
): RenderResult {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  const wrap = (element: ReactNode) => (
    <QueryClientProvider client={client}>{element}</QueryClientProvider>
  );
  const result = locale ? renderWithLocale(wrap(ui), locale, options) : render(wrap(ui), options);
  return { ...result, rerender: (next: ReactNode) => result.rerender(wrap(next)) };
}
