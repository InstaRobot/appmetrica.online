// Mutator для orval-клиента (06-api-contracts.md §9.2): cookie-сессия, базовый URL `/api`,
// `X-Requested-With` на мутациях и конверт ошибок. Единственное место, где разрешён ручной fetch к API.
export class ApiError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId: string,
  ) {
    super(message);
  }
}

type ErrorEnvelope = { error?: { code: string; message: string; request_id: string } };

const MUTATING = new Set(["POST", "PATCH", "PUT", "DELETE"]);

export async function apiFetch<T>(url: string, init: RequestInit = {}): Promise<T> {
  const method = (init.method ?? "GET").toUpperCase();
  const headers = new Headers(init.headers);
  if (MUTATING.has(method)) headers.set("X-Requested-With", "appmetrica-web");
  const res = await fetch(`/api${url}`, { ...init, headers, credentials: "include" });
  if (res.status === 401 && typeof window !== "undefined") {
    window.location.assign(
      `/login?next=${encodeURIComponent(window.location.pathname + window.location.search)}`,
    );
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as ErrorEnvelope | null;
    throw new ApiError(
      res.status,
      body?.error?.code ?? "internal_error",
      body?.error?.message ?? res.statusText,
      body?.error?.request_id ?? "",
    );
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
