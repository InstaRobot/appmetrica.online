// /llms-full.txt: вся английская документация одним Markdown-файлом для ИИ-агентов, в порядке сайдбара.
// Инструкция по интеграции — web/public/llms.txt, этот файл даёт полный код рецептов.
const SITE = "https://appmetrica.online";

export interface LlmsPage {
  title: string;
  description?: string | undefined;
  url: string;
  text: string;
}

const HEADER = [
  "# appmetrica.online documentation",
  "",
  "> Every page of the English documentation as Markdown, in the order of the sidebar, with complete code for every recipe.",
  "",
  `Integration instructions for coding agents: ${SITE}/llms.txt`,
].join("\n");

export function renderLlmsFull(pages: LlmsPage[]): string {
  const sections = pages.map((page) =>
    [`# ${page.title}`, `URL: ${SITE}${page.url}`, page.description, toPlainMarkdown(page.text)]
      .filter(Boolean)
      .join("\n\n"),
  );
  return `${[HEADER, ...sections].join("\n\n---\n\n")}\n`;
}

// Ссылки /docs/… становятся абсолютными, якоря заголовков «## Заголовок [#id]» убираются; блоки кода не трогаются.
function toPlainMarkdown(text: string): string {
  let inFence = false;
  return text
    .trim()
    .split("\n")
    .map((line) => {
      if (/^\s*(```|~~~)/.test(line)) inFence = !inFence;
      if (inFence) return line;
      return line
        .replace(/^(#{1,6} .*?)\s*\[#[\w-]+\]\s*$/, "$1")
        .replaceAll("](/docs", `](${SITE}/docs`);
    })
    .join("\n");
}
