import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import * as zod from "zod";
import { ApiError } from "@/lib/api/fetcher";
import { useZodForm } from "./use-zod-form";

const schema = zod.object({
  name: zod.string().trim().pipe(zod.string().min(1).max(5)),
  kind: zod.enum(["a", "b"]),
});

const messages = {
  name: { too_small: "Введите название", too_big: "Не длиннее 5 символов" },
  kind: "Выберите вид",
};

function setup(onSubmit = vi.fn<(data: zod.output<typeof schema>) => Promise<void>>()) {
  const hook = renderHook(() =>
    useZodForm(schema, {
      defaultValues: { name: "", kind: "" },
      messages,
      serverErrors: { app_name_taken: "name" },
      onSubmit,
    }),
  );
  return { ...hook, onSubmit };
}

describe("useZodForm", () => {
  it("validates a field on blur and leaves untouched fields alone", () => {
    const { result } = setup();

    act(() => result.current.setValue("name", "toolong"));
    expect(result.current.errors).toEqual({});

    act(() => result.current.blur("name"));
    expect(result.current.errors).toEqual({ name: "Не длиннее 5 символов" });
    expect(result.current.field("name")).toMatchObject({
      id: "name",
      "aria-invalid": true,
      "aria-describedby": "name-error",
    });
    expect(result.current.field("kind")["aria-invalid"]).toBeUndefined();
  });

  it("validates every field on submit, focuses the first error and skips onSubmit", async () => {
    const { result, onSubmit } = setup();
    const name = document.createElement("input");
    const kind = document.createElement("button");
    document.body.append(name, kind);
    act(() => {
      result.current.field("name").ref(name);
      result.current.field("kind").ref(kind);
    });

    await act(() => result.current.handleSubmit());

    expect(result.current.errors).toEqual({ name: "Введите название", kind: "Выберите вид" });
    expect(document.activeElement).toBe(name);
    expect(onSubmit).not.toHaveBeenCalled();
    name.remove();
    kind.remove();
  });

  it("revalidates on every change after a failed submit", async () => {
    const { result } = setup();
    await act(() => result.current.handleSubmit());

    act(() => result.current.setValue("kind", "a"));
    expect(result.current.errors.kind).toBeUndefined();
    act(() => result.current.setValue("name", "   "));
    expect(result.current.errors.name).toBe("Введите название");
    act(() => result.current.setValue("name", "ok"));
    expect(result.current.errors.name).toBeUndefined();
  });

  it("submits parsed values and reports submitting while in flight", async () => {
    let resolve = () => {};
    const onSubmit = vi.fn(() => new Promise<void>((r) => (resolve = r)));
    const { result } = setup(onSubmit);
    act(() => {
      result.current.setValue("name", "  Notes ");
      result.current.setValue("kind", "b");
    });

    let pending: Promise<void> = Promise.resolve();
    act(() => {
      pending = result.current.handleSubmit();
    });
    expect(result.current.submitting).toBe(true);
    expect(onSubmit).toHaveBeenCalledWith({ name: "Notes", kind: "b" });

    await act(async () => {
      resolve();
      await pending;
    });
    expect(result.current.submitting).toBe(false);
  });

  it("maps server errors onto fields", () => {
    const { result } = setup();

    let handled = false;
    act(() => {
      handled = result.current.applyServerErrors(
        new ApiError(409, "app_name_taken", "taken", "req-1"),
      );
    });
    expect(handled).toBe(true);
    expect(result.current.errors).toEqual({ name: "Приложение с таким названием уже есть" });

    act(() => {
      handled = result.current.applyServerErrors(
        new ApiError(400, "invalid_request", "bad", "req-2", {
          fields: { kind: "Неизвестный вид", unknown_field: "x" },
        }),
      );
    });
    expect(handled).toBe(true);
    expect(result.current.errors).toEqual({ kind: "Неизвестный вид" });

    act(() => result.current.setValue("kind", "a"));
    expect(result.current.errors.kind).toBeUndefined();
  });

  it("leaves unrelated server errors to the caller", () => {
    const { result } = setup();

    expect(result.current.applyServerErrors(new ApiError(500, "internal_error", "x", "r"))).toBe(
      false,
    );
    expect(
      result.current.applyServerErrors(new ApiError(400, "invalid_request", "x", "r", {})),
    ).toBe(false);
    expect(result.current.applyServerErrors(new Error("boom"))).toBe(false);
    expect(result.current.errors).toEqual({});
  });
});
