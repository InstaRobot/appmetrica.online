// Проверка параметра `next` после входа (защита от open redirect): допускается только путь того же
// origin. Абсолютные URL, protocol-relative `//host` (в том числе после нормализации `/..//host`),
// `/\\host`, схемы и управляющие символы → портфель. Корень «/» — публичный лендинг, после входа туда не ведём.
const ORIGIN = "http://appmetrica.invalid";

// Пробелы и управляющие символы браузеры вырезают при разборе URL: "/\t/evil" превращается в "//evil".
const hasSpaceOrControl = (s: string) =>
  [...s].some((c) => {
    const code = c.charCodeAt(0);
    return code <= 0x20 || code === 0x7f || /\s/.test(c);
  });

export const DEFAULT_AFTER_LOGIN = "/portfolio";

export function safeNext(next: string | string[] | null | undefined): string {
  const value = Array.isArray(next) ? next[0] : next;
  if (!value?.startsWith("/") || value.startsWith("//") || value.startsWith("/\\"))
    return DEFAULT_AFTER_LOGIN;
  if (hasSpaceOrControl(value)) return DEFAULT_AFTER_LOGIN;

  let url: URL;
  try {
    url = new URL(value, ORIGIN);
  } catch {
    return DEFAULT_AFTER_LOGIN;
  }
  if (url.origin !== ORIGIN) return DEFAULT_AFTER_LOGIN;
  // Нормализация точек-сегментов: "/..//evil" и "/%2e%2e//evil" дают pathname "//evil" при том же
  // origin, а браузер прочитает такой Location как protocol-relative URL.
  if (url.pathname.startsWith("//")) return DEFAULT_AFTER_LOGIN;
  // Возврат на /login после входа зациклил бы редирект.
  if (url.pathname === "/login" || url.pathname.startsWith("/login/")) return DEFAULT_AFTER_LOGIN;
  if (url.pathname === "/") return DEFAULT_AFTER_LOGIN;
  return `${url.pathname}${url.search}${url.hash}`;
}
