import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ProfileSearch } from "./profile-search";

describe("ProfileSearch", () => {
  it("submits the trimmed query by Enter and by button", async () => {
    const onSearch = vi.fn();
    render(<ProfileSearch defaultValue="  u:42 " onSearch={onSearch} />);
    const user = userEvent.setup();

    const input = screen.getByRole("searchbox", { name: "install_id или user_id" });
    expect(input).toHaveValue("  u:42 ");
    await user.type(input, "{Enter}");
    expect(onSearch).toHaveBeenLastCalledWith("u:42");

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
