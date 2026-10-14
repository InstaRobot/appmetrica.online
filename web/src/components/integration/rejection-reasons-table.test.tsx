import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { IntegrationReason } from "@/lib/api/gen/model";
import { RejectionReasonsTable } from "./rejection-reasons-table";

const NBSP = " ";

function reason(overrides: Partial<IntegrationReason> = {}): IntegrationReason {
  return {
    code: "event_too_old",
    count: 903,
    last_seen: "2026-10-13T11:20:00Z",
    example_payload:
      '{"insert_id":"0199d6a1-7c2e-7b1a-9f00-3c1d2e4f5a61","name":"session_start","timestamp":"2026-09-01T09:00:00Z"}',
    example_message: "timestamp is older than 7 days",
    ...overrides,
  };
}

const table = () => screen.getByRole("table", { name: "Причины отказов" });

describe("RejectionReasonsTable", () => {
  it("maps codes to descriptions and hints", () => {
    render(
      <RejectionReasonsTable
        timezone="Europe/Moscow"
        reasons={[
          reason(),
          reason({ code: "user_id_looks_like_pii", count: 301, last_seen: "2026-10-13T10:58:00Z" }),
          reason({ code: "brand_new_code", count: 1204 }),
        ]}
      />,
    );

    const rows = within(table()).getAllByRole("row");
    expect(
      within(rows[0] as HTMLElement)
        .getAllByRole("columnheader")
        .map((c) => c.textContent),
    ).toEqual(["Код", "Описание", "Кол-во", "Последний", "Пример"]);

    const first = within(rows[1] as HTMLElement).getAllByRole("cell");
    expect(first[0]).toHaveTextContent("event_too_old");
    expect(first[1]).toHaveTextContent("Событие старше 7 дней");
    expect(first[1]).toHaveTextContent("Удаляйте из офлайн-очереди события старше 7 дней");
    expect(first[2]).toHaveTextContent("903");
    expect(first[3]).toHaveTextContent("14:20");

    const pii = within(rows[2] as HTMLElement).getAllByRole("cell");
    expect(pii[1]).toHaveTextContent("user_id похож на email или телефон");
    expect(pii[3]).toHaveTextContent("13:58");

    const unknown = within(rows[3] as HTMLElement).getAllByRole("cell");
    expect(unknown[1]).toHaveTextContent("brand_new_code");
    expect(unknown[2]).toHaveTextContent(`1${NBSP}204`);
  });

  it("shows the example payload in a popover", async () => {
    const user = userEvent.setup();
    render(
      <RejectionReasonsTable
        timezone="Europe/Moscow"
        reasons={[reason(), reason({ code: "invalid_json", example_payload: "{not json" })]}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Пример payload: event_too_old" }));
    const dialog = await screen.findByRole("dialog", { name: "Пример payload: event_too_old" });
    const code = within(dialog).getByLabelText("Пример payload в JSON");
    expect(code.textContent).toContain('  "name": "session_start",\n');
    expect(within(dialog).getByText("timestamp is older than 7 days")).toBeInTheDocument();

    await user.keyboard("{Escape}");
    await user.click(screen.getByRole("button", { name: "Пример payload: invalid_json" }));
    const raw = await screen.findByRole("dialog", { name: "Пример payload: invalid_json" });
    expect(within(raw).getByLabelText("Пример payload в JSON").textContent).toBe("{not json");
  });

  it("says when there are no rejections", () => {
    render(<RejectionReasonsTable timezone="UTC" reasons={[]} />);
    expect(screen.getByText("Отказов за 24 часа нет ✓")).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
  });
});
