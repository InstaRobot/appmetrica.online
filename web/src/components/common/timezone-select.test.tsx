import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { isTimezone, TimezoneSelect, timezoneLabel } from "./timezone-select";

describe("TimezoneSelect", () => {
  it("shows the current zone with its UTC offset and filters by name", async () => {
    const onChange = vi.fn<(tz: string) => void>();
    const user = userEvent.setup();
    render(<TimezoneSelect id="tz" value="Europe/Moscow" onChange={onChange} />);

    const trigger = screen.getByRole("combobox");
    expect(trigger).toHaveTextContent("Europe/Moscow (UTC+3)");

    await user.click(trigger);
    await user.type(screen.getByPlaceholderText("Найти часовой пояс…"), "new_y");
    const listbox = screen.getByRole("listbox");
    expect(
      within(listbox)
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual([expect.stringMatching(/^America\/New_York \(UTC−[45]\)$/)]);

    await user.click(within(listbox).getByRole("option"));
    expect(onChange).toHaveBeenCalledWith("America/New_York");
  });

  it("offers UTC and says when nothing matches", async () => {
    const user = userEvent.setup();
    render(<TimezoneSelect value="" onChange={() => {}} />);

    expect(screen.getByRole("combobox")).toHaveTextContent("Выберите часовой пояс");
    await user.click(screen.getByRole("combobox"));
    await user.type(screen.getByPlaceholderText("Найти часовой пояс…"), "utc");
    expect(screen.getByRole("option", { name: "UTC (UTC)" })).toBeInTheDocument();

    await user.clear(screen.getByPlaceholderText("Найти часовой пояс…"));
    await user.type(screen.getByPlaceholderText("Найти часовой пояс…"), "Mars");
    expect(screen.getByText("Ничего не найдено")).toBeInTheDocument();
  });
});

describe("timezone helpers", () => {
  it("validates IANA names and formats offsets", () => {
    expect(isTimezone("Europe/Moscow")).toBe(true);
    expect(isTimezone("UTC")).toBe(true);
    expect(isTimezone("Mars/Base")).toBe(false);
    expect(isTimezone("")).toBe(false);
    expect(timezoneLabel("Asia/Kolkata")).toBe("Asia/Kolkata (UTC+5:30)");
    expect(timezoneLabel("UTC")).toBe("UTC (UTC)");
  });
});
