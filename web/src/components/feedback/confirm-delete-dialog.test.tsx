import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDeleteDialog } from "./confirm-delete-dialog";

type HarnessProps = {
  onConfirm: () => void;
  pending?: boolean;
  serverError?: string;
};

function Harness({ onConfirm, pending, serverError }: HarnessProps) {
  const [open, setOpen] = useState(true);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>
        Открыть
      </button>
      <ConfirmDeleteDialog
        open={open}
        onOpenChange={setOpen}
        title="Удалить все данные этого человека?"
        description="Удаление необратимо."
        confirmText="УДАЛИТЬ"
        confirmLabel="Удалить данные"
        onConfirm={onConfirm}
        {...(pending !== undefined ? { pending } : {})}
        {...(serverError !== undefined ? { serverError } : {})}
      />
    </>
  );
}

describe("ConfirmDeleteDialog", () => {
  it("enables confirm only on exact match", async () => {
    const onConfirm = vi.fn();
    render(<Harness onConfirm={onConfirm} />);
    const user = userEvent.setup();

    const dialog = screen.getByRole("alertdialog", { name: "Удалить все данные этого человека?" });
    expect(dialog).toHaveAccessibleDescription(/Удаление необратимо/);
    const input = screen.getByRole("textbox", { name: "Введите УДАЛИТЬ" });
    expect(input).toHaveFocus();

    const confirm = screen.getByRole("button", { name: "Удалить данные" });
    expect(confirm).toBeDisabled();
    await user.type(input, "удалить");
    expect(confirm).toBeDisabled();

    // Enter в поле при несовпадении не подтверждает.
    await user.type(input, "{Enter}");
    expect(onConfirm).not.toHaveBeenCalled();

    await user.clear(input);
    await user.type(input, "  УДАЛИТЬ ");
    expect(confirm).toBeEnabled();
    await user.type(input, "{Enter}");
    expect(onConfirm).toHaveBeenCalledTimes(1);
    await user.click(confirm);
    expect(onConfirm).toHaveBeenCalledTimes(2);
  });

  it("shows the server error under the field", () => {
    render(<Harness onConfirm={vi.fn()} serverError="Слишком много запросов" />);
    const input = screen.getByRole("textbox", { name: "Введите УДАЛИТЬ" });
    expect(input).toHaveAccessibleDescription("Слишком много запросов");
    expect(input).toHaveAttribute("aria-invalid", "true");
  });

  it("disables confirm while pending", async () => {
    render(<Harness onConfirm={vi.fn()} pending />);
    await userEvent.setup().type(screen.getByRole("textbox"), "УДАЛИТЬ");
    expect(screen.getByRole("button", { name: /Удаляем|Удалить данные/ })).toBeDisabled();
  });

  it("resets input on cancel and Esc", async () => {
    const onConfirm = vi.fn();
    render(<Harness onConfirm={onConfirm} />);
    const user = userEvent.setup();

    await user.type(screen.getByRole("textbox"), "УДАЛИТЬ");
    await user.click(screen.getByRole("button", { name: "Отмена" }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Открыть" }));
    expect(screen.getByRole("textbox")).toHaveValue("");
    await user.type(screen.getByRole("textbox"), "УДАЛИТЬ");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Открыть" }));
    expect(screen.getByRole("textbox")).toHaveValue("");
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
