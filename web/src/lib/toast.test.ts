import { beforeEach, describe, expect, it, vi } from "vitest";

const sonner = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), info: vi.fn() }));
vi.mock("sonner", () => ({ toast: sonner }));

// Состояние модуля (ожидание Toaster) — на каждый тест свежее.
async function load() {
  vi.resetModules();
  return import("./toast");
}

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe("toast", () => {
  beforeEach(() => {
    for (const fn of Object.values(sonner)) fn.mockReset();
  });

  it("waits until the Toaster is mounted, then shows queued toasts in order", async () => {
    const { toast, markToasterMounted } = await load();

    toast.success("Сохранено");
    toast.error("Ошибка", { id: "e1" });
    await flush();
    expect(sonner.success).not.toHaveBeenCalled();
    expect(sonner.error).not.toHaveBeenCalled();

    markToasterMounted();
    await vi.waitFor(() => expect(sonner.error).toHaveBeenCalledWith("Ошибка", { id: "e1" }));
    expect(sonner.success).toHaveBeenCalledWith("Сохранено", undefined);
    expect(sonner.success.mock.invocationCallOrder[0]).toBeLessThan(
      sonner.error.mock.invocationCallOrder[0] ?? 0,
    );
  });

  it("shows toasts right away once the Toaster is mounted", async () => {
    const { toast, markToasterMounted } = await load();
    markToasterMounted();

    toast.info("Скопировано", { duration: 1500 });

    await vi.waitFor(() =>
      expect(sonner.info).toHaveBeenCalledWith("Скопировано", { duration: 1500 }),
    );
  });
});
