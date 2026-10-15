import { renderHook } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MessagesProvider } from "@/lib/i18n/client";
import { getMessages } from "@/messages";
import { copyText, useCopyText } from "./clipboard";

const toasts = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@/lib/toast", () => ({ toast: toasts }));

const TEXTS = { copied: "Copied", failed: "Couldn't copy" };

const english = ({ children }: { children: ReactNode }) => (
  <MessagesProvider locale="en" messages={getMessages("en")}>
    {children}
  </MessagesProvider>
);

beforeEach(() => {
  // userEvent подставляет navigator.clipboard, которого нет в jsdom.
  userEvent.setup();
  toasts.success.mockReset();
  toasts.error.mockReset();
});

describe("copyText", () => {
  it("copies the value and announces it with the given texts", async () => {
    await copyText("pk_1", TEXTS);

    expect(await navigator.clipboard.readText()).toBe("pk_1");
    expect(toasts.success).toHaveBeenCalledWith("Copied", { duration: 1500 });
  });

  it("reports a failed copy", async () => {
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));

    await copyText("pk_1", TEXTS);

    expect(toasts.error).toHaveBeenCalledWith("Couldn't copy");
    expect(toasts.success).not.toHaveBeenCalled();
  });
});

describe("useCopyText", () => {
  it("announces the copy in the interface language", async () => {
    const { result } = renderHook(() => useCopyText(), { wrapper: english });

    await result.current("sk_1");

    expect(await navigator.clipboard.readText()).toBe("sk_1");
    expect(toasts.success).toHaveBeenCalledWith("Copied", { duration: 1500 });
  });

  it("speaks Russian under the Russian dictionary", async () => {
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));
    const { result } = renderHook(() => useCopyText());

    await result.current("sk_1");

    expect(toasts.error).toHaveBeenCalledWith("Не удалось скопировать");
  });
});
