import { defineConfig } from "orval";

// Генерация: `pnpm gen:api` (или `make gen-web`). Вывод в src/lib/api/gen/** коммитится и не редактируется руками.
// src/lib/api/fetcher.ts — рукописный mutator (cookie-сессия, базовый URL, конверт ошибок), лежит вне gen/.
export default defineConfig({
  api: {
    input: { target: "../contracts/openapi/api.yaml" },
    output: {
      mode: "tags-split",
      target: "src/lib/api/gen/endpoints",
      schemas: "src/lib/api/gen/model",
      client: "react-query",
      httpClient: "fetch",
      mock: true,
      clean: true,
      formatter: "biome",
      override: {
        mutator: { path: "src/lib/api/fetcher.ts", name: "apiFetch" },
        query: { useQuery: true, useSuspenseQuery: true },
        // Моки не подставляют `undefined` в необязательные поля — иначе конфликт с exactOptionalPropertyTypes.
        mock: { exactOptional: true },
      },
    },
  },
  apiZod: {
    input: { target: "../contracts/openapi/api.yaml" },
    output: {
      mode: "tags-split",
      target: "src/lib/api/gen/zod",
      client: "zod",
      fileExtension: ".zod.ts",
      clean: true,
      formatter: "biome",
    },
  },
});
