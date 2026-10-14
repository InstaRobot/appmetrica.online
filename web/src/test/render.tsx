import { QueryClientProvider } from "@tanstack/react-query";
import { type RenderOptions, type RenderResult, render } from "@testing-library/react";
import type { ReactElement } from "react";
import { makeQueryClient } from "@/lib/query-client";

// render с собственным QueryClient на каждый тест: кэш запросов не протекает между тестами,
// повторы выключены, чтобы ошибочные ответы MSW сразу давали состояние ошибки.
export function renderWithClient(ui: ReactElement, options?: RenderOptions): RenderResult {
  const client = makeQueryClient();
  client.setDefaultOptions({ queries: { ...client.getDefaultOptions().queries, retry: false } });
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>, options);
}
