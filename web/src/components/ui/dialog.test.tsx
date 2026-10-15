import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDeleteDialog } from "@/components/feedback/confirm-delete-dialog";
import { renderWithLocale } from "@/test/rtl";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "./dialog";

function OpenDialog() {
  return (
    <Dialog open>
      <DialogContent>
        <DialogTitle>Title</DialogTitle>
        <DialogDescription>Description</DialogDescription>
        <DialogFooter showCloseButton />
      </DialogContent>
    </Dialog>
  );
}

// Кнопки закрытия диалога подписаны на языке интерфейса: крестик (sr-only) и кнопка футера.
describe("Dialog", () => {
  it("labels the close buttons in English", () => {
    renderWithLocale(<OpenDialog />, "en");

    expect(screen.getAllByRole("button", { name: "Close" })).toHaveLength(2);
  });

  it("labels the close buttons in Russian", () => {
    render(<OpenDialog />);

    expect(screen.getAllByRole("button", { name: "Закрыть" })).toHaveLength(2);
  });
});

// Диалог подтверждения удаления (components/feedback) построен на AlertDialog: подпись поля, отмена и
// состояние удаления — на языке интерфейса.
describe("ConfirmDeleteDialog in English", () => {
  it("speaks English", () => {
    renderWithLocale(
      <ConfirmDeleteDialog
        open
        onOpenChange={vi.fn()}
        title="Delete all data of this person?"
        description="This can't be undone."
        confirmText="DELETE"
        confirmLabel="Delete data"
        pending
        onConfirm={vi.fn()}
      />,
      "en",
    );

    expect(screen.getByRole("textbox", { name: "Type DELETE" })).toHaveFocus();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Deleting…" })).toBeDisabled();
  });
});
