import { createFromSource } from "fumadocs-core/search/server";
import { getAuthState } from "@/lib/api/gen/endpoints/admin-auth/admin-auth";
import { source } from "@/lib/docs/source";

// Поиск Fumadocs по заголовкам и тексту документации (S-13). Индекс — тоже содержимое доков,
// поэтому отдаётся только владельцу с действующей сессией.
const search = createFromSource(source);

export async function GET(request: Request): Promise<Response> {
  const state = await getAuthState();
  if (!state.data.authenticated) {
    return Response.json(
      { error: { code: "unauthenticated", message: "Требуется вход", request_id: "" } },
      { status: 401, headers: { "Cache-Control": "private, no-store" } },
    );
  }
  return search.GET(request);
}
