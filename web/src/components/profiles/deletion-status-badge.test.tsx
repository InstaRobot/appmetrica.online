import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { DeletionStatus } from "@/lib/api/gen/model";
import { renderWithLocale } from "@/test/rtl";
import { DeletionStatusBadge } from "./deletion-status-badge";

describe("DeletionStatusBadge", () => {
  it.each<[DeletionStatus, string, string]>([
    ["pending", "В очереди", "neutral"],
    ["running", "Выполняется", "info"],
    ["done", "Удалено", "success"],
    ["failed", "Ошибка", "danger"],
  ])("shows %s as «%s»", (status, text, tone) => {
    render(<DeletionStatusBadge status={status} />);
    const badge = screen.getByText(text);
    expect(badge).toHaveAttribute("data-tone", tone);
  });
});

describe("DeletionStatusBadge in English", () => {
  it.each<[DeletionStatus, string]>([
    ["pending", "Queued"],
    ["running", "In progress"],
    ["done", "Deleted"],
    ["failed", "Failed"],
  ])("shows %s as «%s»", (status, text) => {
    renderWithLocale(<DeletionStatusBadge status={status} />, "en");
    expect(screen.getByText(text)).toBeInTheDocument();
  });
});
