import { render, screen } from "@testing-library/react";
import { render as renderWithoutProvider } from "@testing-library/react/pure";
import { describe, expect, it } from "vitest";
import { createFormatters } from "@/lib/format";
import { getMessages } from "@/messages";
import { renderWithLocale } from "@/test/rtl";
import { MessagesProvider, useFormat, useLocale, useMessages } from "./client";

function Probe() {
  const m = useMessages();
  const locale = useLocale();
  const f = useFormat();
  return (
    <p>
      {locale}|{m.common.save}|{f.formatInt(12480)}|{String(f === createFormatters(locale))}
    </p>
  );
}

describe("MessagesProvider", () => {
  it("gives components the dictionary, the locale and its formatters", () => {
    renderWithoutProvider(
      <MessagesProvider locale="en" messages={getMessages("en")}>
        <Probe />
      </MessagesProvider>,
    );
    expect(screen.getByText("en|Save|12,480|true")).toBeInTheDocument();
  });

  it("fails loudly when a component is rendered outside the provider", () => {
    expect(() => renderWithoutProvider(<Probe />)).toThrow(/MessagesProvider/);
  });
});

describe("test render helpers", () => {
  it("render wraps components in the Russian dictionary by default", () => {
    const { container } = render(<Probe />);
    // textContent, а не getByText: матчер нормализует U+00A0 в DOM, но не в строке ожидания.
    expect(container.textContent).toBe("ru|Сохранить|12\u00a0480|true");
  });

  it("renderWithLocale switches the dictionary", () => {
    renderWithLocale(<Probe />, "en");
    expect(screen.getByText("en|Save|12,480|true")).toBeInTheDocument();
  });
});
