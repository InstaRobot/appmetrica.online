import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { DocsTable } from "./docs-table";

describe("DocsTable", () => {
  it("wraps the table in a region the keyboard can scroll", async () => {
    render(
      <DocsTable>
        <tbody>
          <tr>
            <td>ios</td>
          </tr>
        </tbody>
      </DocsTable>,
    );
    const region = screen.getByRole("table").parentElement;

    expect(region).toHaveClass("overflow-auto");
    await userEvent.tab();
    expect(region).toHaveFocus();
  });
});
