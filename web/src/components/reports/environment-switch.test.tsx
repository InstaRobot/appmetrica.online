import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { EnvironmentSwitch } from "./environment-switch";
import { STRETCH_GROUP, STRETCH_ITEM, TOUCH_TOGGLE } from "./responsive";

describe("EnvironmentSwitch", () => {
  it("switches the environment and keeps one selected", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<EnvironmentSwitch value="production" onChange={onChange} />);

    expect(screen.getByRole("radio", { name: "production" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await user.click(screen.getByRole("radio", { name: "production" }));
    expect(onChange).not.toHaveBeenCalled();
    await user.click(screen.getByRole("radio", { name: "development" }));
    expect(onChange).toHaveBeenCalledWith("development");
  });

  // jsdom не считает CSS: телефонная вёрстка проверяется по брейкпоинт-классам.
  it("stretches over the phone width with 44 px options", () => {
    render(<EnvironmentSwitch value="production" onChange={vi.fn()} />);

    expect(screen.getByRole("radiogroup", { name: "Окружение" })).toHaveClass(STRETCH_GROUP);
    for (const radio of screen.getAllByRole("radio")) {
      expect(radio).toHaveClass(TOUCH_TOGGLE, STRETCH_ITEM);
    }
  });
});
