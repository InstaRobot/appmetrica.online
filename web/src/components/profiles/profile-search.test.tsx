import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { ProfileSearch } from "./profile-search";

describe("ProfileSearch", () => {
  it("submits the trimmed query by Enter and by button", async () => {
    const onSearch = vi.fn();
    render(<ProfileSearch defaultValue="  u:42 " onSearch={onSearch} />);
    const user = userEvent.setup();

    const input = screen.getByRole("searchbox", { name: "install_id или user_id" });
    expect(input).toHaveValue("  u:42 ");
    await user.type(input, "{Enter}");
    // Первая отправка дожидается загрузки чанка со схемой (lib/forms/schemas.ts).
    await waitFor(() => expect(onSearch).toHaveBeenLastCalledWith("u:42"));

    await user.clear(input);
    await user.type(input, " 5d8e ");
    await user.click(screen.getByRole("button", { name: "Найти" }));
    expect(onSearch).toHaveBeenLastCalledWith("5d8e");
  });

  it("requires a value", async () => {
    const onSearch = vi.fn();
    render(<ProfileSearch onSearch={onSearch} />);
    const user = userEvent.setup();

    await user.type(screen.getByRole("searchbox"), "   ");
    await user.click(screen.getByRole("button", { name: "Найти" }));
    expect(onSearch).not.toHaveBeenCalled();
    expect(screen.getByRole("searchbox")).toHaveAccessibleDescription(
      "Введите install_id или user_id",
    );
  });

  it("limits the query to 200 characters", async () => {
    const onSearch = vi.fn();
    render(<ProfileSearch defaultValue={"a".repeat(201)} onSearch={onSearch} />);
    await userEvent.setup().click(screen.getByRole("button", { name: "Найти" }));
    expect(onSearch).not.toHaveBeenCalled();
    expect(screen.getByRole("searchbox")).toHaveAttribute("aria-invalid", "true");
  });

  it("disables the button while searching", () => {
    render(<ProfileSearch defaultValue="42" onSearch={vi.fn()} pending />);
    expect(screen.getByRole("button", { name: "Найти" })).toBeDisabled();
  });
});

describe("ProfileSearch in English", () => {
  it("labels, hints and validates in English", async () => {
    const onSearch = vi.fn();
    renderWithLocale(<ProfileSearch onSearch={onSearch} />, "en");
    const user = userEvent.setup();

    const input = screen.getByRole("searchbox", { name: "install_id or user_id" });
    expect(input).toHaveAttribute("placeholder", "install_id or user_id");
    expect(input).toHaveAccessibleDescription(
      "Exact match search. A person_key like u:… or i:… works too.",
    );
    await user.type(input, "   ");
    await user.click(screen.getByRole("button", { name: "Find" }));
    await waitFor(() =>
      expect(input).toHaveAccessibleDescription("Enter an install_id or user_id"),
    );
    expect(onSearch).not.toHaveBeenCalled();
  });

  it("limits the query to 200 characters in English", async () => {
    renderWithLocale(<ProfileSearch defaultValue={"a".repeat(201)} onSearch={vi.fn()} />, "en");
    await userEvent.setup().click(screen.getByRole("button", { name: "Find" }));
    await waitFor(() =>
      expect(screen.getByRole("searchbox")).toHaveAccessibleDescription("200 characters at most"),
    );
  });
});
