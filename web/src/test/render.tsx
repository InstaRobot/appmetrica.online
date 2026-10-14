import { QueryClientProvider } from "@tanstack/react-query";
import { type RenderOptions, type RenderResult, render } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";
import { makeQueryClient } from "@/lib/query-client";

// render с собственным QueryClient на каждый тест: кэш запросов не протекает между тестами,
// повторы выключены, чтобы ошибочные ответы MSW сразу давали состояние ошибки. rerender оставляет тот же
// клиент: кэш между перерисовками сохраняется, как в приложении.
export function renderWithClient(ui: ReactElement, options?: RenderOptions): RenderResult {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  const wrap = (element: ReactNode) => (
    <QueryClientProvider client={client}>{element}</QueryClientProvider>
  );
  const result = render(wrap(ui), options);
  return { ...result, rerender: (next: ReactNode) => result.rerender(wrap(next)) };
}
