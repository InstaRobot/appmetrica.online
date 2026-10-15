import defaultMdxComponents from "fumadocs-ui/mdx";
import { DocsBody, DocsDescription, DocsPage, DocsTitle } from "fumadocs-ui/page";
import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { APIPage } from "@/components/docs/api-page";
import { source } from "@/lib/docs/source";

// S-13 `/docs/[[...slug]]`: MDX-гайд или страница операции API reference, доступна без входа.
export default async function DocsRoute({ params }: PageProps<"/[lang]/docs/[[...slug]]">) {
  const { slug } = await params;
  const page = source.getPage(slug);
  if (!page) notFound();

  if (page.data.type === "openapi") {
    return (
      <DocsPage full>
        <DocsTitle>{page.data.title}</DocsTitle>
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
      <DocsTitle>{page.data.title}</DocsTitle>
      <DocsDescription>{page.data.description}</DocsDescription>
      <DocsBody>
        <MDX components={defaultMdxComponents} />
      </DocsBody>
    </DocsPage>
  );
}

export async function generateMetadata({
  params,
}: PageProps<"/[lang]/docs/[[...slug]]">): Promise<Metadata> {
  const { slug } = await params;
  const page = source.getPage(slug);
  if (!page) return { title: "Документация" };
  return {
    title: page.data.title,
    ...(page.data.description ? { description: page.data.description } : {}),
  };
}
