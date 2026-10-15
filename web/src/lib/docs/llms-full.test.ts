import { describe, expect, it } from "vitest";
import { renderLlmsFull } from "./llms-full";

const pages = [
  {
    title: "Quick start",
    description: "How to start sending events.",
    url: "/docs",
    text: "Read [Sessions](/docs/concepts/sessions#client-rules) first.\n\n## Event platform [#platform]\n\nText.",
  },
  {
    title: "Sessions",
    url: "/docs/concepts/sessions",
    text: "```bash\ncurl https://i.analytics.example.com/v1/events\n```",
  },
];

describe("renderLlmsFull", () => {
  const text = renderLlmsFull(pages);

  it("starts with the llms.txt title, a summary and a link to the integration guide", () => {
    expect(text).toMatch(/^# appmetrica\.online documentation\n\n> .+\n/);
    expect(text).toContain("https://appmetrica.online/llms.txt");
  });

  it("keeps the order of the pages and gives each its absolute URL", () => {
    const quickStart = text.indexOf("# Quick start\n\nURL: https://appmetrica.online/docs\n");
    const sessions = text.indexOf(
      "# Sessions\n\nURL: https://appmetrica.online/docs/concepts/sessions\n",
    );
    expect(quickStart).toBeGreaterThan(0);
    expect(sessions).toBeGreaterThan(quickStart);
  });

  it("adds the page description when there is one", () => {
    expect(text).toContain(
      "URL: https://appmetrica.online/docs\n\nHow to start sending events.\n\n",
    );
  });

  it("makes docs links absolute", () => {
    expect(text).toContain(
      "[Sessions](https://appmetrica.online/docs/concepts/sessions#client-rules)",
    );
    expect(text).not.toContain("](/docs");
  });

  it("drops the heading anchors of the docs syntax", () => {
    expect(text).toContain("## Event platform\n");
    expect(text).not.toContain("[#platform]");
  });

  it("leaves code blocks as they are", () => {
    expect(text).toContain("```bash\ncurl https://i.analytics.example.com/v1/events\n```");
  });
});
