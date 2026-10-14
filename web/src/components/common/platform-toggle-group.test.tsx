import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Platform } from "@/lib/api/gen/model";
import { PlatformToggleGroup } from "./platform-toggle-group";

function Controlled({
  initial = [],
  onValueChange,
}: {
  initial?: Platform[];
  onValueChange: (value: Platform[]) => void;
}) {
  const [value, setValue] = useState(initial);
  return (
    <PlatformToggleGroup
      aria-label="Платформы"
      value={value}
      onValueChange={(next) => {
        setValue(next);
        onValueChange(next);
      }}
    />
  );
}

describe("PlatformToggleGroup", () => {
  it("offers every platform in enum order with a visible label", () => {
    render(<PlatformToggleGroup aria-label="Платформы" value={[]} onValueChange={vi.fn()} />);

    const group = screen.getByRole("group", { name: "Платформы" });
    const chips = screen.getAllByRole("button");
    expect(chips.map((chip) => chip.textContent)).toEqual([
      "iOS",
      "Android",
      "Веб",
      "macOS",
      "Windows",
      "Linux",
      "Бот",
      "Бэкенд",
      "Другое",
    ]);
    for (const chip of chips) {
      expect(group).toContainElement(chip);
      expect(chip).toHaveAttribute("aria-pressed", "false");
      expect(chip.querySelector("svg")).toHaveAttribute("aria-hidden", "true");
    }
  });

  it("toggles with the keyboard", async () => {
    const onValueChange = vi.fn<(value: Platform[]) => void>();
    const user = userEvent.setup();
    render(<Controlled onValueChange={onValueChange} />);

    await user.tab();
    expect(screen.getByRole("button", { name: "iOS" })).toHaveFocus();
    await user.tab();
    const android = screen.getByRole("button", { name: "Android" });
    expect(android).toHaveFocus();
    await user.keyboard(" ");

    expect(android).toHaveAttribute("aria-pressed", "true");
    expect(onValueChange).toHaveBeenLastCalledWith(["android"]);

    await user.keyboard("{Enter}");
    expect(android).toHaveAttribute("aria-pressed", "false");
    expect(onValueChange).toHaveBeenLastCalledWith([]);
  });

  it("keeps the enum order whatever the click order", async () => {
    const onValueChange = vi.fn<(value: Platform[]) => void>();
    const user = userEvent.setup();
    render(<Controlled onValueChange={onValueChange} />);

    await user.click(screen.getByRole("button", { name: "Веб" }));
    await user.click(screen.getByRole("button", { name: "Бот" }));
    await user.click(screen.getByRole("button", { name: "iOS" }));

    expect(onValueChange).toHaveBeenLastCalledWith(["ios", "web", "bot"]);
  });

  it("removes a platform when its chip is pressed again", async () => {
    const onValueChange = vi.fn<(value: Platform[]) => void>();
    const user = userEvent.setup();
    render(<Controlled initial={["ios", "web"]} onValueChange={onValueChange} />);

    expect(screen.getByRole("button", { name: "iOS" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Веб" })).toHaveAttribute("aria-pressed", "true");
    await user.click(screen.getByRole("button", { name: "iOS" }));

    expect(onValueChange).toHaveBeenLastCalledWith(["web"]);
  });

  it("disables every chip and passes the description to the group", async () => {
    const onValueChange = vi.fn();
    const user = userEvent.setup();
    render(
      <>
        <p id="platforms-hint">Подсказка</p>
        <PlatformToggleGroup
          aria-label="Платформы"
          aria-describedby="platforms-hint"
          value={["android"]}
          onValueChange={onValueChange}
          disabled
        />
      </>,
    );

    expect(screen.getByRole("group", { name: "Платформы" })).toHaveAccessibleDescription(
      "Подсказка",
    );
    for (const chip of screen.getAllByRole("button")) expect(chip).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "iOS" }));
    expect(onValueChange).not.toHaveBeenCalled();
  });

  it("forwards the ref to the first chip", () => {
    let first: HTMLButtonElement | null = null;
    render(
      <PlatformToggleGroup
        aria-label="Платформы"
        value={[]}
        onValueChange={vi.fn()}
        ref={(el) => {
          first = el;
        }}
      />,
    );

    expect(first).toBe(screen.getByRole("button", { name: "iOS" }));
  });
});
