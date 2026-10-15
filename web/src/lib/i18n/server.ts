import { cookies, headers } from "next/headers";
import { LOCALE_COOKIE, type Locale, resolveLocale } from "./locale";

// Язык панели, входа и демо: сохранённый выбор → язык браузера → EN. Читает cookie и заголовки запроса,
// поэтому вызывается под Suspense (cacheComponents). Публичные страницы берут язык из params.lang.
export async function getRequestLocale(): Promise<Locale> {
  const [jar, list] = await Promise.all([cookies(), headers()]);
  return resolveLocale({
    cookie: jar.get(LOCALE_COOKIE)?.value,
    acceptLanguage: list.get("accept-language"),
  });
}
