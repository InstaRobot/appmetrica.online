import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/fetcher";
import ErrorPage from "./error";
import GlobalError from "./global-error";

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
});

describe("global-error", () => {
  it("repeats the error text with its own html shell", () => {
    const reset = vi.fn();
    // global-error рендерит собственные <html>/<body>; в jsdom проверяем содержимое.
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    render(<GlobalError error={new Error("fatal")} reset={reset} />);
    spy.mockRestore();

    expect(screen.getByRole("heading", { level: 1, name: "Что-то пошло не так" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Попробовать снова" })).toBeVisible();
  });
});
