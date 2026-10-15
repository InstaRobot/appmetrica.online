import { flattenTree } from "fumadocs-core/page-tree";
import { renderLlmsFull } from "@/lib/docs/llms-full";
import { source } from "@/lib/docs/source";

// /llms-full.txt — английские MDX-гайды одним Markdown-файлом (страницы API reference генерируются из OpenAPI
// и сюда не входят). Прокси пропускает *.txt, поэтому адрес не переписывается на /en.
export async function GET() {
  const order = flattenTree(source.getPageTree("en").children).map((item) => item.url);
  const pages = await Promise.all(
    source.getPages("en").flatMap(({ url, data }) =>
      data.type === "openapi" || !order.includes(url)
        ? []
        : [
            data.getText("processed").then((text) => ({
              title: data.title ?? "",
              description: data.description,
              url,
              text,
            })),
          ],
    ),
  );
  pages.sort((a, b) => order.indexOf(a.url) - order.indexOf(b.url));
  return new Response(renderLlmsFull(pages), {
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
}
