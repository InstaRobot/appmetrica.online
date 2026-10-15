import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import type { Platform } from "@/lib/api/gen/model";
import { renderWithLocale } from "@/test/rtl";
import { PlatformSwitcher } from "./platform-switcher";
import { STRETCH_GROUP, STRETCH_ITEM, TOUCH_TOGGLE } from "./responsive";

type Props = {
  appPlatforms: Platform[];
  observed: string[];
  initial?: Platform | null;
  onChange?: (platform: Platform | null) => void;
};

function Controlled({ appPlatforms, observed, initial = null, onChange }: Props) {
  const [value, setValue] = useState<Platform | null>(initial);
  return (
    <PlatformSwitcher
      appPlatforms={appPlatforms}
      observed={observed}
      value={value}
      onChange={(next) => {
        setValue(next);
        onChange?.(next);
      }}
    />
  );
}

const options = () => screen.getAllByRole("radio").map((r) => r.textContent);

describe("PlatformSwitcher", () => {
  it("lists explicit and observed platforms", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <Controlled
        appPlatforms={["ios", "android"]}
        observed={["web", "android", "ios"]}
        onChange={onChange}
      />,
    );

    expect(screen.getByRole("radiogroup", { name: "Платформа" })).toBeInTheDocument();
    expect(options()).toEqual(["Все", "iOS", "Android", "Веб"]);
    expect(screen.getByRole("radio", { name: "Все" })).toHaveAttribute("aria-checked", "true");

    await user.click(screen.getByRole("radio", { name: "Android" }));
    expect(onChange).toHaveBeenLastCalledWith("android");
    expect(screen.getByRole("radio", { name: "Android" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("radio", { name: "Все" })).toHaveAttribute("aria-checked", "false");

    await user.click(screen.getByRole("radio", { name: "Все" }));
    expect(onChange).toHaveBeenLastCalledWith(null);
  });

  it("hides for a single platform", () => {
    render(<Controlled appPlatforms={["web"]} observed={["web"]} />);
    expect(screen.queryByRole("radiogroup", { name: "Платформа" })).not.toBeInTheDocument();
  });

  it("moves with arrow keys", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <Controlled appPlatforms={["ios", "android", "web"]} observed={[]} onChange={onChange} />,
    );

    // В группу попадает только выбранная опция (roving tabindex).
    await user.tab();
    expect(screen.getByRole("radio", { name: "Все" })).toHaveFocus();
    expect(screen.getByRole("radio", { name: "iOS" })).toHaveAttribute("tabindex", "-1");

    await user.keyboard("{ArrowRight}");
    expect(onChange).toHaveBeenLastCalledWith("ios");
    expect(screen.getByRole("radio", { name: "iOS" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("radio", { name: "iOS" })).toHaveFocus();

    await user.keyboard("{End}");
    expect(onChange).toHaveBeenLastCalledWith("web");
    expect(screen.getByRole("radio", { name: "Веб" })).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("radio", { name: "Веб" })).toHaveFocus();

    // С последней опции стрелка вправо переходит на первую, Home — тоже на первую.
    await user.keyboard("{ArrowRight}");
    expect(onChange).toHaveBeenLastCalledWith(null);
    await user.keyboard("{ArrowLeft}");
    expect(onChange).toHaveBeenLastCalledWith("web");
    await user.keyboard("{Home}");
    expect(onChange).toHaveBeenLastCalledWith(null);
    expect(screen.getByRole("radio", { name: "Все" })).toHaveFocus();
  });

  it("orders options by the platform enum and ignores unknown observed values", () => {
    render(
      <Controlled appPlatforms={["web", "ios"]} observed={["symbian", "", "android", "other"]} />,
    );
    expect(options()).toEqual(["Все", "iOS", "Android", "Веб", "Другое"]);
  });

  it("keeps the selected platform so it can be reset", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    // Выбранная платформа (из URL) не явная и не наблюдалась: без неё переключатель бы исчез, а фильтр остался.
    render(
      <Controlled appPlatforms={["web"]} observed={[]} initial="android" onChange={onChange} />,
    );

    expect(options()).toEqual(["Все", "Android", "Веб"]);
    expect(screen.getByRole("radio", { name: "Android" })).toHaveAttribute("aria-checked", "true");
    await user.click(screen.getByRole("radio", { name: "Все" }));
    expect(onChange).toHaveBeenLastCalledWith(null);
  });

  it("does not report a click on the selected option", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(<Controlled appPlatforms={["ios", "android"]} observed={[]} onChange={onChange} />);
    await user.click(screen.getByRole("radio", { name: "Все" }));
    expect(onChange).not.toHaveBeenCalled();
  });
});

// jsdom не считает CSS: телефонная вёрстка проверяется по брейкпоинт-классам.
describe("PlatformSwitcher on a phone", () => {
  it("stretches over the phone width with 44 px options", () => {
    render(<Controlled appPlatforms={["ios", "android"]} observed={[]} />);

    expect(screen.getByRole("radiogroup", { name: "Платформа" })).toHaveClass(STRETCH_GROUP);
    for (const radio of screen.getAllByRole("radio")) {
      expect(radio).toHaveClass(TOUCH_TOGGLE, STRETCH_ITEM);
    }
  });
});

describe("PlatformSwitcher in English", () => {
  it("names the group and the option for all platforms in English", () => {
    renderWithLocale(<Controlled appPlatforms={["ios", "android"]} observed={[]} />, "en");
    expect(screen.getByRole("radiogroup", { name: "Platform" })).toBeInTheDocument();
    expect(options()).toEqual(["All", "iOS", "Android"]);
  });
});
