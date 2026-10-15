import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { WaitingForFirstEvent } from "./waiting-for-first-event";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

beforeEach(() => {
  toasts.success.mockReset();
});

const props = { appName: "Notes iOS", audienceHref: "/apps/a1", timezone: "Europe/Moscow" };

describe("WaitingForFirstEvent", () => {
  it("turns green when the first event arrives", () => {
    const { rerender } = render(<WaitingForFirstEvent {...props} firstEventAt={null} />);

    const status = screen.getByRole("status");
    expect(status).toHaveAttribute("aria-live", "polite");
    expect(status).toHaveTextContent("Ждём первое событие… Проверяем каждые 3 секунды.");
    expect(screen.queryByRole("link", { name: "Открыть Аудиторию" })).not.toBeInTheDocument();

    rerender(<WaitingForFirstEvent {...props} firstEventAt="2026-10-13T11:32:05Z" />);
    expect(screen.getByRole("status")).toHaveTextContent(
      "Первое событие получено 14:32:05 — Notes iOS подключено!",
    );
    expect(screen.getByRole("link", { name: "Открыть Аудиторию" })).toHaveAttribute(
      "href",
      "/apps/a1",
    );
    expect(toasts.success).toHaveBeenCalledTimes(1);
    expect(toasts.success).toHaveBeenCalledWith("Первое событие получено");

    rerender(<WaitingForFirstEvent {...props} firstEventAt="2026-10-13T11:32:05Z" />);
    expect(toasts.success).toHaveBeenCalledTimes(1);
  });

  // Телефон: кнопка переносится под текст и остаётся целью нажатия 44 px.
  it("wraps the audience button under the text on phones", () => {
    render(<WaitingForFirstEvent {...props} firstEventAt="2026-10-13T11:32:05Z" />);

    const open = screen.getByRole("link", { name: "Открыть Аудиторию" });
    expect(open).toHaveClass("h-11", "md:h-8");
    expect(open.parentElement).toHaveClass("flex-wrap");
  });

  it("shows success without a toast when the event was already there", () => {
    render(<WaitingForFirstEvent {...props} firstEventAt="2026-10-13T11:32:05Z" />);
    expect(screen.getByRole("status")).toHaveTextContent("Первое событие получено 14:32:05");
    expect(toasts.success).not.toHaveBeenCalled();
  });

  it("waits and turns green in English", () => {
    const { rerender } = renderWithLocale(
      <WaitingForFirstEvent {...props} firstEventAt={null} />,
      "en",
    );

    expect(screen.getByRole("status", { name: "First event" })).toHaveTextContent(
      "Waiting for the first event… Checking every 3 seconds.",
    );

    rerender(<WaitingForFirstEvent {...props} firstEventAt="2026-10-13T11:32:05Z" />);
    expect(screen.getByRole("status", { name: "First event" })).toHaveTextContent(
      "First event received at 14:32:05 — Notes iOS is connected!",
    );
    expect(toasts.success).toHaveBeenCalledWith("First event received");
    expect(screen.getByRole("link", { name: "Open Audience" })).toHaveAttribute("href", "/apps/a1");
  });
});
