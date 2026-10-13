import { QueryClient } from "@tanstack/react-query";
import { ApiError } from "@/lib/api/fetcher";

// Политика повторов (14-ux-ui-spec.md §8.3): 4xx не повторяются, 503 storage_unavailable — 3 раза
// с задержкой 1, 2, 4 с, остальные ошибки (5xx, сеть) — один раз.
export function shouldRetry(failureCount: number, error: unknown): boolean {
  if (error instanceof ApiError) {
    if (error.status >= 400 && error.status < 500) return false;
    if (error.status === 503 && error.code === "storage_unavailable") return failureCount < 3;
  }
  return failureCount < 1;
}

export function retryDelay(attempt: number): number {
  return 1000 * 2 ** attempt;
}

export function makeQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { staleTime: 30_000, retry: shouldRetry, retryDelay },
      mutations: { retry: false },
    },
  });
}
