import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Suspense } from "react";
import { describe, expect, it, vi } from "vitest";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./lazy-select";

describe("lazy Select", () => {
  it("shows the fallback until the chunk loads, then works as a Select", async () => {
    const onValueChange = vi.fn();
    const user = userEvent.setup();
    render(
      <Suspense fallback={<p>Загрузка поля</p>}>
        <Select onValueChange={onValueChange}>
          <SelectTrigger aria-label="Платформа">
            <SelectValue placeholder="Любая платформа" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ios">iOS</SelectItem>
            <SelectItem value="web">Веб</SelectItem>
          </SelectContent>
        </Select>
      </Suspense>,
    );

    expect(screen.getByText("Загрузка поля")).toBeInTheDocument();
    const trigger = await screen.findByRole("combobox", { name: "Платформа" });
    expect(trigger).toHaveTextContent("Любая платформа");

    await user.click(trigger);
    await user.click(await screen.findByRole("option", { name: "Веб" }));

    expect(onValueChange).toHaveBeenCalledWith("web");
    expect(trigger).toHaveTextContent("Веб");
  });
});
