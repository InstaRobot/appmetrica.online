import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { THEME_STORAGE_KEY } from "@/lib/theme";
import { renderWithLocale } from "@/test/rtl";
import { ThemeToggle } from "./theme-toggle";

const root = () => document.documentElement;

beforeEach(() => {
  localStorage.clear();
  root().dataset.theme = "light";
  root().classList.remove("dark");
});

describe("ThemeToggle", () => {
  it("switches light → dark → light and keeps the label in sync", async () => {
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.click(screen.getByRole("button", { name: "Включить тёмную тему" }));

    expect(root().dataset.theme).toBe("dark");
    expect(root().classList.contains("dark")).toBe(true);
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");

    await user.click(screen.getByRole("button", { name: "Включить светлую тему" }));

    expect(root().dataset.theme).toBe("light");
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("light");
    expect(screen.getByRole("button", { name: "Включить тёмную тему" })).toBeInTheDocument();
  });

  it("starts from the theme already applied by the init script", () => {
    root().dataset.theme = "dark";

    render(<ThemeToggle />);

    expect(screen.getByRole("button", { name: "Включить светлую тему" })).toBeInTheDocument();
  });

  it("speaks English", async () => {
    const user = userEvent.setup();
    renderWithLocale(<ThemeToggle />, "en");

    await user.click(screen.getByRole("button", { name: "Switch to dark theme" }));
    expect(screen.getByRole("button", { name: "Switch to light theme" })).toBeInTheDocument();
  });
});
