import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "./table";

function Keys({ stackOnMobile = false }: { stackOnMobile?: boolean }) {
  return (
    <Table stackOnMobile={stackOnMobile}>
      <caption>Ключи</caption>
      <TableHeader>
        <TableRow>
          <TableHead>Ключ</TableHead>
          <TableHead>Создан</TableHead>
          <TableHead>
            <span className="sr-only">Действия</span>
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow>
          <TableCell>pk_live…Ab0</TableCell>
          <TableCell label="Создан">12 окт 2026</TableCell>
          <TableCell>
            <button type="button">Отозвать</button>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  );
}

// Таблица-карточки на телефоне: роли проставлены явно, ячейки подписаны data-label, шапка скрыта визуально.
describe("Table stackOnMobile", () => {
  it("keeps table semantics with explicit roles", () => {
    render(<Keys stackOnMobile />);

    const table = screen.getByRole("table", { name: "Ключи" });
    expect(table).toHaveAttribute("role", "table");
    const [head, body] = within(table).getAllByRole("rowgroup");
    expect(head?.tagName).toBe("THEAD");
    expect(head).toHaveClass("max-md:sr-only");
    expect(body?.tagName).toBe("TBODY");
    for (const row of within(table).getAllByRole("row")) expect(row).toHaveAttribute("role", "row");
    expect(within(table).getAllByRole("columnheader")).toHaveLength(3);
    for (const header of within(table).getAllByRole("columnheader")) {
      expect(header).toHaveAttribute("role", "columnheader");
    }
    for (const cell of within(table).getAllByRole("cell"))
      expect(cell).toHaveAttribute("role", "cell");
  });

  it("labels cells and spans the unlabelled ones across the card", () => {
    render(<Keys stackOnMobile />);

    const [key, created, actions] = screen.getAllByRole("cell");
    expect(created).toHaveAttribute("data-label", "Создан");
    expect(created).toHaveClass("max-md:before:content-[attr(data-label)]");
    expect(created).not.toHaveClass("max-md:col-span-full");
    expect(key).not.toHaveAttribute("data-label");
    expect(key).toHaveClass("max-md:col-span-full");
    expect(actions).toHaveClass("max-md:col-span-full");
    expect(screen.getAllByRole("row")[1]).toHaveClass("max-md:grid", "max-md:border");
  });

  it("scrolls only from md when stacked", () => {
    const { container } = render(<Keys stackOnMobile />);

    const scroller = container.querySelector('[data-slot="table-container"]');
    expect(scroller).toHaveClass("md:overflow-x-auto");
    expect(scroller).not.toHaveClass("overflow-x-auto");
  });

  it("stays a plain scrolling table by default", () => {
    const { container } = render(<Keys />);

    const table = screen.getByRole("table", { name: "Ключи" });
    expect(table).not.toHaveAttribute("role");
    expect(container.querySelector('[data-slot="table-container"]')).toHaveClass("overflow-x-auto");
    expect(container.querySelector("thead")).not.toHaveClass("max-md:sr-only");
    const cells = screen.getAllByRole("cell");
    for (const cell of cells) {
      expect(cell).not.toHaveAttribute("role");
      expect(cell.className).not.toMatch(/max-md:/);
    }
    expect(cells[1]).toHaveAttribute("data-label", "Создан");
  });
});
