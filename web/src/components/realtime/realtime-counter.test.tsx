import { act, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { RealtimeCounter } from "./realtime-counter";

const UPDATED = "2026-10-13T11:32:05Z";

beforeEach(() => {
  vi.useFakeTimers({ now: new Date(UPDATED) });
});

afterEach(() => {
  vi.useRealTimers();
});

const region = () => screen.getByRole("region", { name: "Активны сейчас" });

describe("RealtimeCounter", () => {
  it("shows active users with the update time in the app time zone", () => {
    render(
      <RealtimeCounter value={1284} updatedAt={UPDATED} timezone="Europe/Moscow" paused={false} />,
    );

    expect(within(region()).getByText(/^1\s284$/)).toBeInTheDocument();
    expect(within(region()).getByText("за последние 30 мин")).toBeInTheDocument();
    expect(within(region()).getByText("обновлено 14:32:05")).toBeInTheDocument();
    expect(region().querySelector("[data-live]")).toHaveAttribute("data-live", "live");
  });

  it("shows paused and stale indicators", () => {
    const { rerender } = render(
      <RealtimeCounter value={5} updatedAt={UPDATED} timezone="Europe/Moscow" paused />,
    );
    expect(within(region()).getByText("на паузе")).toBeInTheDocument();
    expect(region().querySelector("[data-live]")).toHaveAttribute("data-live", "paused");

    rerender(
      <RealtimeCounter
        value={5}
        updatedAt={UPDATED}
        timezone="Europe/Moscow"
        paused={false}
        stale
      />,
    );
    expect(within(region()).getByText("нет связи")).toBeInTheDocument();
    expect(region().querySelector("[data-live]")).toHaveAttribute("data-live", "stale");
  });

  it("announces the value at most once per 30 seconds", () => {
    const { rerender } = render(
      <RealtimeCounter value={10} updatedAt={UPDATED} timezone="Europe/Moscow" paused={false} />,
    );
    const live = () => region().querySelector("[aria-live='polite']");
    expect(live()).toHaveTextContent("Активны сейчас: 10");

    act(() => vi.advanceTimersByTime(5000));
    rerender(
      <RealtimeCounter value={12} updatedAt={UPDATED} timezone="Europe/Moscow" paused={false} />,
    );
    expect(within(region()).getByText("12")).toBeInTheDocument();
    expect(live()).toHaveTextContent("Активны сейчас: 10");

    act(() => vi.advanceTimersByTime(25_000));
    expect(live()).toHaveTextContent("Активны сейчас: 12");

    rerender(
      <RealtimeCounter value={15} updatedAt={UPDATED} timezone="Europe/Moscow" paused={false} />,
    );
    act(() => vi.advanceTimersByTime(29_000));
    expect(live()).toHaveTextContent("Активны сейчас: 12");
    act(() => vi.advanceTimersByTime(1000));
    expect(live()).toHaveTextContent("Активны сейчас: 15");
  });
});

describe("RealtimeCounter in English", () => {
  it("shows the counter and its state in English", () => {
    const { rerender } = renderWithLocale(
      <RealtimeCounter value={1284} updatedAt={UPDATED} timezone="Europe/Moscow" paused={false} />,
      "en",
    );
    const counter = screen.getByRole("region", { name: "Active now" });
    expect(within(counter).getByText("1,284")).toBeInTheDocument();
    expect(within(counter).getByText("in the last 30 min")).toBeInTheDocument();
    expect(within(counter).getByText("updated 14:32:05")).toBeInTheDocument();
    expect(counter.querySelector("[aria-live='polite']")).toHaveTextContent("Active now: 1,284");

    rerender(<RealtimeCounter value={1284} updatedAt={UPDATED} timezone="Europe/Moscow" paused />);
    expect(within(counter).getByText("paused")).toBeInTheDocument();

    rerender(
      <RealtimeCounter
        value={1284}
        updatedAt={UPDATED}
        timezone="Europe/Moscow"
        paused={false}
        stale
      />,
    );
    expect(within(counter).getByText("no connection")).toBeInTheDocument();
  });
});
