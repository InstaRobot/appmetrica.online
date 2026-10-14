import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CodeBlock } from "./code-block";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

const CODE = 'const ENDPOINT = "https://i.appmetrica.online/v1/events";\nsessionStart();\n';

describe("CodeBlock", () => {
  it("renders focusable code with a label and highlights it", async () => {
    render(<CodeBlock code={CODE} lang="javascript" ariaLabel="Пример кода на JavaScript" />);

    const pre = screen.getByLabelText("Пример кода на JavaScript");
    expect(pre.tagName).toBe("PRE");
    expect(pre).toHaveAttribute("tabindex", "0");
    expect(pre).toHaveStyle({ maxHeight: "480px" });
    expect(pre.textContent).toBe(CODE);

    // Подсветка shiki приходит асинхронно и не меняет текст.
    await waitFor(() => expect(pre.querySelector("span[style]")).not.toBeNull(), {
      timeout: 5000,
    });
    expect(pre.textContent?.trimEnd()).toBe(CODE.trimEnd());
  });

  it("copies the code", async () => {
    const user = userEvent.setup();
    render(<CodeBlock code={CODE} lang="javascript" ariaLabel="Пример" maxHeight={320} />);

    expect(screen.getByLabelText("Пример")).toHaveStyle({ maxHeight: "320px" });
    await user.click(screen.getByRole("button", { name: "Копировать код" }));
    expect(await navigator.clipboard.readText()).toBe(CODE);
    expect(toasts.success).toHaveBeenCalledWith("Скопировано", { duration: 1500 });
  });
});
