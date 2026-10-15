import defaultMdxComponents from "fumadocs-ui/mdx";
import type { ComponentProps } from "react";
import type { Locale } from "@/lib/i18n/locale";

const MdxLink = defaultMdxComponents.a;

// В MDX внутренние ссылки пишутся как /docs/…; на русской странице они ведут на /ru/docs/…, иначе
// каждый переход проходил бы через редирект proxy.
export function docsHref(href: string | undefined, locale: Locale): string | undefined {
  if (locale !== "ru" || !href) return href;
  return href === "/docs" || href.startsWith("/docs/") || href.startsWith("/docs#")
    ? `/ru${href}`
    : href;
}

export function docsLink(locale: Locale) {
  return function DocsLink({ href, ...props }: ComponentProps<typeof MdxLink>) {
    return <MdxLink href={docsHref(href, locale)} {...props} />;
  };
}
