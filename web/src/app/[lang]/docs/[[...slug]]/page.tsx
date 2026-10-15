import defaultMdxComponents from "fumadocs-ui/mdx";
import { DocsBody, DocsDescription, DocsPage, DocsTitle } from "fumadocs-ui/page";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { APIPage } from "@/components/docs/api-page";
import { docsLink } from "@/components/docs/docs-link";
import { DocsTable } from "@/components/docs/docs-table";
import { source } from "@/lib/docs/source";
import { isLocale, type Locale } from "@/lib/i18n/locale";
import { getMessages } from "@/messages";

const localeOf = (lang: string): Locale => (isLocale(lang) ? lang : "en");

// Заголовок Fumadocs (1.75em) наследует межстрочный интервал текста 20 px: на узком экране перенесённые
// строки налезают друг на друга. Десктоп (xl) не меняется — там заголовки помещаются в строку.
const TITLE = "max-xl:leading-[1.2]";

// S-13 `/docs/[[...slug]]`: MDX-гайд или страница операции API reference, доступна без входа.
export default async function DocsRoute({ params }: PageProps<"/[lang]/docs/[[...slug]]">) {
  const { slug, lang } = await params;
  const locale = localeOf(lang);
  const page = source.getPage(slug, locale);
  if (!page) notFound();

  if (page.data.type === "openapi") {
    return (
      <DocsPage full>
        <DocsTitle className={TITLE}>{page.data.title}</DocsTitle>
        <DocsDescription>{page.data.description}</DocsDescription>
        <DocsBody>
          <APIPage {...page.data.getOpenAPIPageProps()} />
        </DocsBody>
      </DocsPage>
    );
  }

  const MDX = page.data.body;
  return (
    <DocsPage toc={page.data.toc} full={page.data.full ?? false}>
      <DocsTitle className={TITLE}>{page.data.title}</DocsTitle>
      <DocsDescription>{page.data.description}</DocsDescription>
      <DocsBody>
        <MDX components={{ ...defaultMdxComponents, a: docsLink(locale), table: DocsTable }} />
      </DocsBody>
    </DocsPage>
  );
}

export async function generateMetadata({
  params,
}: PageProps<"/[lang]/docs/[[...slug]]">): Promise<Metadata> {
  const { slug, lang } = await params;
  const locale = localeOf(lang);
  const page = source.getPage(slug, locale);
  if (!page) return { title: getMessages(locale).docs.navTitle };
  return {
    title: page.data.title,
    ...(page.data.description ? { description: page.data.description } : {}),
  };
}
