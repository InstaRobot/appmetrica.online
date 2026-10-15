import { act, render, screen } from "@testing-library/react/pure";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/fetcher";
import ErrorPage from "./error";
import GlobalError from "./global-error";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

// Корневые границы ошибок стоят вне MessagesProvider: render из «/pure» без словаря, язык — из <html lang>,
// который inline-скрипт ставит до первой отрисовки.
beforeEach(() => {
  document.documentElement.lang = "ru";
});
afterEach(() => {
  document.documentElement.lang = "";
});

describe("S-14b error", () => {
  it("renders the full-page error with retry and focuses the heading", async () => {
    const reset = vi.fn();
    render(<ErrorPage error={new Error("boom")} reset={reset} />);

    const heading = screen.getByRole("heading", { level: 1, name: "Что-то пошло не так" });
    expect(heading).toHaveFocus();
    expect(
      screen.getByText("Произошла непредвиденная ошибка. Попробуйте обновить страницу."),
    ).toBeVisible();
    expect(screen.getByRole("link", { name: "В портфель" })).toHaveAttribute("href", "/portfolio");
    expect(screen.queryByText(/Код запроса/)).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Попробовать снова" }));
    expect(reset).toHaveBeenCalledOnce();
  });

  it("shows the request_id of an ApiError", () => {
    const error = new ApiError(500, "internal_error", "Ошибка", "01JREQUEST");
    render(<ErrorPage error={error} reset={() => {}} />);

    expect(screen.getByText("Код запроса: 01JREQUEST")).toBeVisible();
  });

  it("does not leak the raw error message", () => {
    render(<ErrorPage error={new Error("pq: relation apps does not exist")} reset={() => {}} />);

    expect(screen.queryByText(/relation apps/)).not.toBeInTheDocument();
  });

  it("speaks English on an English page and follows <html lang>", async () => {
    const user = userEvent.setup();
    document.documentElement.lang = "en";
    const error = new ApiError(500, "internal_error", "boom", "01JREQUEST");
    render(<ErrorPage error={error} reset={() => {}} />);

    expect(screen.getByRole("heading", { level: 1, name: "Something went wrong" })).toHaveFocus();
    expect(
      screen.getByText("An unexpected error occurred. Try refreshing the page."),
    ).toBeVisible();
    expect(screen.getByText("Request ID: 01JREQUEST")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Copy request ID" }));
    expect(toasts.success).toHaveBeenLastCalledWith("Copied", { duration: 1500 });
    expect(screen.getByRole("button", { name: "Try again" })).toBeVisible();
    expect(screen.getByRole("link", { name: "Go to portfolio" })).toHaveAttribute(
      "href",
      "/portfolio",
    );

    act(() => {
      document.documentElement.lang = "ru";
    });
    expect(
      await screen.findByRole("heading", { level: 1, name: "Что-то пошло не так" }),
    ).toBeInTheDocument();
  });

  it("falls back to English when <html lang> is unknown", () => {
    document.documentElement.lang = "";
    render(<ErrorPage error={new Error("boom")} reset={() => {}} />);

    expect(screen.getByRole("heading", { level: 1, name: "Something went wrong" })).toBeVisible();
  });
});

// Телефон (от 360 px): поля 16 px, кнопки — цели нажатия 44 px и переносятся, если не помещаются в строку.
describe("S-14b error on a phone", () => {
  it("keeps the page readable at 360 px", () => {
    render(<ErrorPage error={new Error("boom")} reset={() => {}} />);

    const retry = screen.getByRole("button", { name: "Попробовать снова" });
    expect(retry).toHaveClass("h-11", "md:h-9");
    expect(screen.getByRole("link", { name: "В портфель" })).toHaveClass("h-11", "md:h-9");
    expect(retry.parentElement).toHaveClass("flex-wrap", "justify-center");
    expect(screen.getByRole("heading", { level: 1 }).parentElement).toHaveClass("px-4", "md:px-8");
  });
});

describe("global-error", () => {
  it("keeps the fallback readable on a phone", () => {
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(<GlobalError error={new Error("fatal")} reset={() => {}} />);
    spy.mockRestore();

    // Стили встроенные (globals.css может не загрузиться): поля 16 px и перенос кнопок, а размер цели
    // нажатия 44 px задаёт встроенный <style> только для телефона.
    expect(screen.getByRole("main")).toHaveStyle({ paddingLeft: "16px", paddingRight: "16px" });
    const retry = screen.getByRole("button", { name: "Попробовать снова" });
    expect(retry.parentElement).toHaveStyle({ flexWrap: "wrap", justifyContent: "center" });
    expect(retry).toHaveClass("global-error-action");
    expect(screen.getByRole("link", { name: "В портфель" })).toHaveClass("global-error-action");
  });

  it("repeats the error text with its own html shell", () => {
    const reset = vi.fn();
    // global-error рендерит собственные <html>/<body>; в jsdom проверяем содержимое.
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(<GlobalError error={new Error("fatal")} reset={reset} />);
    spy.mockRestore();

    expect(screen.getByRole("heading", { level: 1, name: "Что-то пошло не так" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Попробовать снова" })).toBeVisible();
  });

  it("speaks English on an English page", () => {
    document.documentElement.lang = "en";
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(<GlobalError error={new Error("fatal")} reset={() => {}} />);
    spy.mockRestore();

    expect(screen.getByRole("heading", { level: 1, name: "Something went wrong" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Try again" })).toBeVisible();
    expect(screen.getByRole("link", { name: "Go to portfolio" })).toHaveAttribute(
      "href",
      "/portfolio",
    );
    expect(document.documentElement).toHaveAttribute("lang", "en");
  });
});
