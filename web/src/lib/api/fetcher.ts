// Mutator для orval-клиента (06-api-contracts.md §9.2, 07-project-structure.md §4). Единственное место,
// где разрешён ручной fetch к API.
// - Браузер ходит на `/api` того же origin (rewrite Next → Go API), сервер — напрямую на API_INTERNAL_URL
//   и пробрасывает cookie входящего запроса.
// - `X-Requested-With: appmetrica-web` на мутациях (CSRF-защита admin API).
// - Успех возвращается в форме, которую ждут сгенерированные функции: `{ data, status, headers }`.
// - Ошибка — исключение ApiError с полями конверта Error; 401 отправляет на `/login`.
import type { ErrorErrorDetails } from "@/lib/api/gen/model";
import { DEMO_COOKIE, hasDemoCookie } from "@/lib/demo-mode";

export class ApiError extends Error {
  override readonly name = "ApiError";

  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly requestId: string,
    readonly details?: ErrorErrorDetails,
  ) {
    super(message);
  }
}

type ErrorEnvelope = {
  error?: { code?: unknown; message?: unknown; request_id?: unknown; details?: unknown };
};

const MUTATING = new Set(["POST", "PATCH", "PUT", "DELETE"]);
const DEFAULT_API_INTERNAL_URL = "http://localhost:8080";

const isBrowser = () => typeof window !== "undefined";

// Демо-режим: ответы даёт статичный датасет lib/demo (отдельный чанк, грузится только в демо).
async function inDemo(): Promise<boolean> {
  if (isBrowser()) return hasDemoCookie(document.cookie);
  const { cookies } = await import("next/headers");
  return (await cookies()).get(DEMO_COOKIE)?.value === "1";
}

async function serverRequest(url: string, headers: Headers): Promise<string> {
  const { cookies } = await import("next/headers");
  const cookie = (await cookies()).toString();
  if (cookie && !headers.has("cookie")) headers.set("cookie", cookie);
  return `${process.env.API_INTERNAL_URL || DEFAULT_API_INTERNAL_URL}${url}`;
}

// В браузере навигация асинхронная, поэтому вызывающий код всё равно получает ApiError;
// на сервере redirect() бросает NEXT_REDIRECT и до ApiError дело не доходит.
async function redirectToLogin(): Promise<void> {
  if (isBrowser()) {
    const next = window.location.pathname + window.location.search;
    window.location.assign(`/login?next=${encodeURIComponent(next)}`);
    return;
  }
  const { redirect } = await import("next/navigation");
  redirect("/login");
}

async function toApiError(res: Response): Promise<ApiError> {
  const body = (await res.json().catch(() => null)) as ErrorEnvelope | null;
  const e = body?.error;
  if (!e || typeof e.code !== "string") {
    return new ApiError(res.status, `http_${res.status}`, res.statusText || "Ошибка запроса", "");
  }
  return new ApiError(
    res.status,
    e.code,
    typeof e.message === "string" ? e.message : res.statusText,
    typeof e.request_id === "string" ? e.request_id : "",
    e.details && typeof e.details === "object" ? (e.details as ErrorErrorDetails) : undefined,
  );
}

export async function apiFetch<T>(url: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method ?? "GET").toUpperCase();
  const headers = new Headers(init.headers);
  if (MUTATING.has(method)) headers.set("X-Requested-With", "appmetrica-web");

  const demo = await inDemo();

  let res: Response;
  try {
    if (demo) {
      const { demoFetch } = await import("@/lib/demo/handler");
      res = await demoFetch(url, { ...init, method, headers });
    } else {
      const target = isBrowser() ? `/api${url}` : await serverRequest(url, headers);
      res = await fetch(target, { ...init, method, headers, credentials: "include" });
    }
  } catch (err) {
    // Отмена запроса (смена фильтров, размонтирование) — не сетевая ошибка; TanStack Query её ожидает как есть.
    if (init.signal?.aborted) throw err;
    throw new ApiError(0, "network_error", "Нет соединения с сервером", "");
  }

  if (!res.ok) {
    const error = await toApiError(res);
    if (res.status === 401 && !demo) await redirectToLogin();
    throw error;
  }

  const text = res.status === 204 ? "" : await res.text();
  const data: unknown = text ? JSON.parse(text) : undefined;
  return { data, status: res.status, headers: res.headers } as T;
}
