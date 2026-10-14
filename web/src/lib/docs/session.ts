import { redirect } from "next/navigation";
import { cache } from "react";
import { getAuthState } from "@/lib/api/gen/endpoints/admin-auth/admin-auth";

// requireOwner — серверная проверка сессии для документации (S-13, FR-023). proxy.ts пропускает
// запрос с любой cookie am_session, поэтому валидность сессии спрашиваем у API. cache() — один
// запрос на рендер, даже если проверяют и layout, и страница.
export const requireOwner = cache(async (): Promise<void> => {
  const state = await getAuthState();
  if (!state.data.authenticated) redirect("/login?next=/docs");
});
