import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithLocale } from "@/test/rtl";
import { CopyField } from "./copy-field";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

beforeEach(() => {
  toasts.success.mockReset();
  toasts.error.mockReset();
});

describe("CopyField", () => {
  it("copies the value and announces it", async () => {
    const user = userEvent.setup();
    render(<CopyField value="pk_7Hq2x9AbLm3Np4Qr5St6Uv7Wx8Yz9Ab0" label="write-ключ" />);

    const input = screen.getByRole("textbox", { name: "write-ключ" });
    expect(input).toHaveValue("pk_7Hq2x9AbLm3Np4Qr5St6Uv7Wx8Yz9Ab0");
    expect(input).toHaveAttribute("readonly");
    expect(input).toHaveClass("font-mono");

    await user.click(screen.getByRole("button", { name: "Копировать write-ключ" }));
    expect(await navigator.clipboard.readText()).toBe("pk_7Hq2x9AbLm3Np4Qr5St6Uv7Wx8Yz9Ab0");
    expect(toasts.success).toHaveBeenCalledWith("Скопировано", { duration: 1500 });
  });

  it("masks secrets but copies the full value", async () => {
    const user = userEvent.setup();
    const secret = "sk_AbCdEfGh1234567890abcdefghijklmnopqrstuvwx";
    render(<CopyField value={secret} label="секрет" masked mono={false} />);

    const input = screen.getByRole("textbox", { name: "секрет" });
    expect(input).toHaveValue("sk_AbCdEfGh••••");
    expect(input).not.toHaveClass("font-mono");

    await user.click(screen.getByRole("button", { name: "Копировать секрет" }));
    expect(await navigator.clipboard.readText()).toBe(secret);
  });

  it("reports a failed copy", async () => {
    const user = userEvent.setup();
    render(<CopyField value="x" label="ключ" />);
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));

    await user.click(screen.getByRole("button", { name: "Копировать ключ" }));
    expect(toasts.error).toHaveBeenCalledWith("Не удалось скопировать");
    expect(toasts.success).not.toHaveBeenCalled();
  });

  it("speaks English", async () => {
    const user = userEvent.setup();
    renderWithLocale(<CopyField value="pk_123" label="write key" />, "en");

    await user.click(screen.getByRole("button", { name: "Copy write key" }));
    expect(toasts.success).toHaveBeenCalledWith("Copied", { duration: 1500 });

    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));
    await user.click(screen.getByRole("button", { name: "Copy write key" }));
    expect(toasts.error).toHaveBeenCalledWith("Couldn't copy");
  });
});
