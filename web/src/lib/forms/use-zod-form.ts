import type { FormEvent } from "react";
import type * as zod from "zod";

type Field<S extends zod.ZodObject> = keyof zod.input<S> & string;

export type FormValues<S extends zod.ZodObject> = {
  [K in Field<S>]: zod.input<S>[K] extends string ? string : zod.input<S>[K];
};
export type FormErrors<S extends zod.ZodObject> = Partial<Record<Field<S>, string>>;
export type FieldMessages = string | Partial<Record<string, string>>;

type UseZodFormOptions<S extends zod.ZodObject> = {
  defaultValues: FormValues<S>;
  messages?: Partial<Record<Field<S>, FieldMessages>>;
  serverErrors?: Partial<Record<string, Field<S>>>;
  onSubmit: (data: zod.output<S>) => void | Promise<void>;
};

// TODO(T-037)
export function useZodForm<S extends zod.ZodObject>(_schema: S, options: UseZodFormOptions<S>) {
  return {
    values: options.defaultValues,
    errors: {} as FormErrors<S>,
    submitting: false,
    setValue: <K extends Field<S>>(_name: K, _value: FormValues<S>[K]) => {},
    blur: (_name: Field<S>) => {},
    handleSubmit: async (_event?: FormEvent) => {},
    applyServerErrors: (_error: unknown): boolean => false,
    field: (name: Field<S>) => ({
      id: name as string,
      "aria-invalid": undefined as true | undefined,
      "aria-describedby": undefined as string | undefined,
      ref: (_el: HTMLElement | null) => {},
    }),
    errorId: (name: Field<S>) => `${name}-error`,
  };
}
