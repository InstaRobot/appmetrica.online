import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/fetcher";
import { ArchivedBanner } from "./archived-banner";
import { ErrorState } from "./error-state";

describe("ErrorState", () => {
  it("shows inline API error with request id and retry", async () => {
    const onRetry = vi.fn();
    render(
      <ErrorState
        error={new ApiError(503, "storage_unavailable", "clickhouse down", "01JREQ")}
        onRetry={onRetry}
      />,
    );

    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent(
      "Хранилище данных временно недоступно. Повторим автоматически.",
    );
    expect(alert).not.toHaveTextContent("clickhouse down");
    expect(screen.getByText("Код запроса: 01JREQ")).toBeVisible();
    await userEvent.click(screen.getByRole("button", { name: "Повторить" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("prefers explicit correlation id and hides retry without handler", () => {
    render(
      <ErrorState
        error={{ code: "something_new", message: "x", request_id: "01JA" }}
        correlationId="01JB"
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent("Что-то пошло не так");
    expect(screen.getByText("Код запроса: 01JB")).toBeVisible();
    expect(screen.queryByRole("button", { name: "Повторить" })).toBeNull();
  });

  it("renders full-page variant with focused h1", () => {
    render(<ErrorState error={new Error("boom")} variant="full-page" onRetry={vi.fn()} />);

    const heading = screen.getByRole("heading", { level: 1, name: "Что-то пошло не так" });
    expect(heading).toHaveFocus();
    expect(screen.getByRole("button", { name: "Попробовать снова" })).toBeVisible();
    expect(screen.getByRole("link", { name: "В портфель" })).toHaveAttribute("href", "/portfolio");
    expect(screen.queryByText(/Код запроса/)).toBeNull();
  });
});

describe("ArchivedBanner", () => {
  it("explains that events are rejected", () => {
    render(<ArchivedBanner />);

    expect(screen.getByRole("status")).toHaveTextContent(
      "Приложение в архиве: события отклоняются с кодом app_archived.",
    );
  });
});
