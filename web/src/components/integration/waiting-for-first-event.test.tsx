import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
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

  it("shows success without a toast when the event was already there", () => {
    render(<WaitingForFirstEvent {...props} firstEventAt="2026-10-13T11:32:05Z" />);
    expect(screen.getByRole("status")).toHaveTextContent("Первое событие получено 14:32:05");
    expect(toasts.success).not.toHaveBeenCalled();
  });
});
