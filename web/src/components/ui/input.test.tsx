import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Input } from "./input";
import { Textarea } from "./textarea";

// iOS увеличивает страницу при фокусе на поле со шрифтом меньше 16 px: на телефоне — 16 px, на десктопе
// шрифт прежний. Высота поля на телефоне — 44 px, как у кнопок.
describe("text fields on phones", () => {
  it("use a 16 px font and a 44 px height below md", () => {
    render(<Input aria-label="Название" />);
    const input = screen.getByRole("textbox", { name: "Название" });
    expect(input).toHaveClass("max-md:text-[16px]", "max-md:h-11", "md:text-sm");
  });

  it("use a 16 px font in text areas below md", () => {
    render(<Textarea aria-label="Комментарий" />);
    expect(screen.getByRole("textbox", { name: "Комментарий" })).toHaveClass("max-md:text-[16px]");
  });
});
