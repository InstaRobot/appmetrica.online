import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FilterBar } from "./filter-bar";

const options = {
  platform: [{ value: "ios", count: 900 }],
  app_version: [
    { value: "2.1.1", count: 600 },
    { value: "2.1.0", count: 300 },
  ],
  country: [{ value: "RU", count: 700 }],
};

// Код popover грузится лениво: ждём настоящий триггер (с aria-expanded), а не кнопку-заглушку.
const filterTrigger = () => screen.findByRole("button", { name: "Фильтр", expanded: false });

describe("FilterBar", () => {
  it("shows active filters as removable chips", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <FilterBar
        value={{ platform: ["ios"], app_version: ["2.1.0", "2.1.1"] }}
        onChange={onChange}
        options={options}
      />,
    );

    expect(screen.getByText("Платформа: ios")).toBeInTheDocument();
    expect(screen.getByText("Версия приложения: 2.1.0, 2.1.1")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Убрать фильтр «Платформа»" }));
    expect(onChange).toHaveBeenLastCalledWith({ app_version: ["2.1.0", "2.1.1"] });

    await user.click(screen.getByRole("button", { name: "Сбросить фильтры" }));
    expect(onChange).toHaveBeenLastCalledWith({});
  });

  it("adds and removes values from the list", async () => {
    const onChange = vi.fn();
    const onOpenChange = vi.fn();
    const user = userEvent.setup();
    const { rerender } = render(
      <FilterBar value={{}} onChange={onChange} options={options} onOpenChange={onOpenChange} />,
    );
    expect(screen.queryByRole("button", { name: "Сбросить фильтры" })).not.toBeInTheDocument();

    await user.click(await filterTrigger());
    expect(onOpenChange).toHaveBeenCalledWith(true);
    const option = await screen.findByRole("option", { name: /^2\.1\.0/ });
    expect(option).toHaveTextContent("300");
    await user.click(option);
    expect(onChange).toHaveBeenLastCalledWith({ app_version: ["2.1.0"] });

    rerender(
      <FilterBar
        value={{ app_version: ["2.1.0"], country: ["RU"] }}
        onChange={onChange}
        options={options}
      />,
    );
    await user.click(await filterTrigger());
    expect(await screen.findByRole("option", { name: /^2\.1\.0/ })).toHaveAttribute(
      "data-checked",
      "true",
    );
    await user.click(await screen.findByRole("option", { name: /^RU/ }));
    expect(onChange).toHaveBeenLastCalledWith({ app_version: ["2.1.0"] });
  });

  it("says that values are loading", async () => {
    const user = userEvent.setup();
    render(<FilterBar value={{}} onChange={vi.fn()} options={{}} loading />);
    await user.click(await filterTrigger());
    expect(screen.getByText("Загружаем значения…")).toBeInTheDocument();
  });

  it("filters the list by the search text", async () => {
    const user = userEvent.setup();
    render(<FilterBar value={{}} onChange={vi.fn()} options={options} />);
    await user.click(await filterTrigger());
    await user.type(screen.getByRole("combobox"), "zzz");
    expect(screen.getByText("Ничего не найдено")).toBeInTheDocument();
  });

  // Платформу на страницах приложения выбирает PlatformSwitcher, а не список фильтров.
  it("offers only the given dimensions and keeps the others on reset", async () => {
    const onChange = vi.fn();
    const user = userEvent.setup();
    render(
      <FilterBar
        dims={["app_version", "os_name", "country"]}
        value={{ platform: ["ios"], country: ["RU"] }}
        onChange={onChange}
        options={options}
      />,
    );

    expect(screen.queryByText("Платформа: ios")).not.toBeInTheDocument();
    expect(screen.getByText("Страна: RU")).toBeInTheDocument();

    await user.click(await filterTrigger());
    expect(await screen.findByRole("option", { name: /^2\.1\.0/ })).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: "Платформа" })).not.toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /^ios/ })).not.toBeInTheDocument();
    await user.click(screen.getByRole("option", { name: /^2\.1\.0/ }));
    expect(onChange).toHaveBeenLastCalledWith({
      platform: ["ios"],
      country: ["RU"],
      app_version: ["2.1.0"],
    });

    await user.click(screen.getByRole("button", { name: "Сбросить фильтры" }));
    expect(onChange).toHaveBeenLastCalledWith({ platform: ["ios"] });
  });

  it("offers no reset when only hidden dimensions are set", () => {
    render(
      <FilterBar
        dims={["app_version", "os_name", "country"]}
        value={{ platform: ["ios"] }}
        onChange={vi.fn()}
        options={options}
      />,
    );
    expect(screen.queryByRole("button", { name: "Сбросить фильтры" })).not.toBeInTheDocument();
  });
});
