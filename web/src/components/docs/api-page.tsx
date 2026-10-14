"use client";

import { createOpenAPIPage } from "fumadocs-openapi/ui";

// Страница операции API reference (S-13). Playground выключен: справочник только описывает
// контракт, запросы с ключами шлют клиенты по рецептам, а не браузер владельца.
export const APIPage = createOpenAPIPage({ playground: { enabled: false } });
