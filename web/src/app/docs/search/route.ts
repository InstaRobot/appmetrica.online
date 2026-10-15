import { createFromSource } from "fumadocs-core/search/server";
import { source } from "@/lib/docs/source";

// Поиск Fumadocs по заголовкам и тексту документации (S-13). Документация публичная, поэтому и
// индекс отдаётся без сессии.
export const { GET } = createFromSource(source);
