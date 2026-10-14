import { type FormEvent, useCallback, useEffect, useRef, useState } from "react";
import type * as zod from "zod";
import { ApiError } from "@/lib/api/fetcher";
import { messageFor } from "@/lib/errors";

// Хук форм дашборда (14-ux-ui-spec.md §7.1) вместо react-hook-form: правила — Zod-схемы orval из
// contracts/openapi/api.yaml, тексты ошибок — по полю и коду issue Zod. Ошибка поля показывается на blur,
// при отправке — у всех полей; после первой неудачной отправки поле проверяется на каждый ввод.
// Схема передаётся загрузчиком `() => import("@/lib/forms/schemas").then(…)`: zod (≈ 30 KB gz) не входит
// в initial JS страниц с формами (бюджет NFR-006, T-044) и загружается при монтировании формы. Пока схема
// не загружена, blur и ввод ошибок не показывают, а отправка дожидается схемы.

type Field<S extends zod.ZodObject> = keyof zod.input<S> & string;

// Строковые поля (включая enum) принимают любую строку: незаполненный select — "".
export type FormValues<S extends zod.ZodObject> = {
  [K in Field<S>]: zod.input<S>[K] extends string ? string : zod.input<S>[K];
};
export type FormErrors<S extends zod.ZodObject> = Partial<Record<Field<S>, string>>;
/** Один текст на любое нарушение или тексты по коду issue Zod (too_small, too_big, invalid_value, custom…). */
export type FieldMessages = string | Partial<Record<string, string>>;

type UseZodFormOptions<S extends zod.ZodObject> = {
  defaultValues: FormValues<S>;
  messages?: Partial<Record<Field<S>, FieldMessages>>;
  /** Код ошибки API → поле, к которому относится текст из lib/errors.ts (409 app_name_taken → name). */
  serverErrors?: Partial<Record<string, Field<S>>>;
  onSubmit: (data: zod.output<S>) => void | Promise<void>;
};

function issueText(issue: zod.core.$ZodIssue, messages: FieldMessages | undefined): string {
  if (typeof messages === "string") return messages;
  return messages?.[issue.code] ?? issue.message;
}

export type SchemaSource<S extends zod.ZodObject> = S | (() => Promise<S>);

export function useZodForm<S extends zod.ZodObject>(
  source: SchemaSource<S>,
  options: UseZodFormOptions<S>,
) {
  const [values, setValues] = useState(options.defaultValues);
  const [errors, setErrors] = useState<FormErrors<S>>({});
  const [submitting, setSubmitting] = useState(false);
  // Актуальные значения и опции для обработчиков, вызванных до перерисовки (select: onValueChange → blur).
  const valuesRef = useRef(values);
  const optionsRef = useRef(options);
  optionsRef.current = options;
  const submittedRef = useRef(false);
  const elements = useRef(new Map<Field<S>, HTMLElement>());
  // Поля формы — ключи defaultValues: FormValues требует значение для каждого поля схемы.
  const [fields] = useState(() => Object.keys(options.defaultValues) as Field<S>[]);

  const schemaRef = useRef(typeof source === "function" ? undefined : source);
  const loadingRef = useRef<Promise<S> | undefined>(undefined);
  const sourceRef = useRef(source);
  const loadSchema = useCallback((): Promise<S> => {
    const loaded = schemaRef.current;
    if (loaded) return Promise.resolve(loaded);
    const load = sourceRef.current as () => Promise<S>;
    loadingRef.current ??= load().then(
      (schema) => {
        schemaRef.current = schema;
        return schema;
      },
      (error: unknown) => {
        // Чанк не загрузился (сеть): следующая отправка формы попробует снова.
        loadingRef.current = undefined;
        throw error;
      },
    );
    return loadingRef.current;
  }, []);
  useEffect(() => {
    loadSchema().catch(() => {});
  }, [loadSchema]);

  const validate = useCallback((schema: S, current: FormValues<S>) => {
    const result = schema.safeParse(current);
    const next: FormErrors<S> = {};
    if (!result.success) {
      for (const issue of result.error.issues) {
        const name = issue.path[0] as Field<S>;
        if (next[name] === undefined) {
          next[name] = issueText(issue, optionsRef.current.messages?.[name]);
        }
      }
    }
    return { result, errors: next };
  }, []);

  const showFieldError = useCallback(
    (name: Field<S>) => {
      const schema = schemaRef.current;
      if (!schema) return;
      const message = validate(schema, valuesRef.current).errors[name];
      setErrors((prev) => {
        const next = { ...prev };
        if (message === undefined) delete next[name];
        else next[name] = message;
        return next;
      });
    },
    [validate],
  );

  const setValue = useCallback(
    <K extends Field<S>>(name: K, value: FormValues<S>[K]) => {
      valuesRef.current = { ...valuesRef.current, [name]: value };
      setValues(valuesRef.current);
      if (submittedRef.current) {
        showFieldError(name);
      } else {
        // Серверная ошибка поля снимается при его изменении.
        setErrors((prev) => {
          if (prev[name] === undefined) return prev;
          const next = { ...prev };
          delete next[name];
          return next;
        });
      }
    },
    [showFieldError],
  );

  const blur = useCallback((name: Field<S>) => showFieldError(name), [showFieldError]);

  const focusFirst = useCallback(
    (failed: FormErrors<S>) => {
      const first = fields.find((name) => failed[name] !== undefined);
      if (first) elements.current.get(first)?.focus();
    },
    [fields],
  );

  const handleSubmit = useCallback(
    async (event?: FormEvent) => {
      event?.preventDefault();
      submittedRef.current = true;
      const schema = schemaRef.current ?? (await loadSchema());
      const { result, errors: failed } = validate(schema, valuesRef.current);
      setErrors(failed);
      if (!result.success) {
        focusFirst(failed);
        return;
      }
      setSubmitting(true);
      try {
        await optionsRef.current.onSubmit(result.data);
      } finally {
        setSubmitting(false);
      }
    },
    [validate, focusFirst, loadSchema],
  );

  // applyServerErrors раскладывает ошибку API по полям: код из serverErrors или 400 invalid_request с
  // details.fields. Возвращает false, если ошибка не относится к полям (её показывает вызывающий код).
  const applyServerErrors = useCallback(
    (error: unknown): boolean => {
      if (!(error instanceof ApiError)) return false;
      const next: FormErrors<S> = {};
      const field = optionsRef.current.serverErrors?.[error.code];
      if (field) {
        next[field] = messageFor(error.code);
      } else if (error.code === "invalid_request") {
        const reported = error.details?.fields;
        if (reported && typeof reported === "object") {
          for (const [name, text] of Object.entries(reported)) {
            if (fields.includes(name as Field<S>) && typeof text === "string") {
              next[name as Field<S>] = text;
            }
          }
        }
      }
      if (Object.keys(next).length === 0) return false;
      setErrors(next);
      focusFirst(next);
      return true;
    },
    [fields, focusFirst],
  );

  const errorId = (name: Field<S>) => `${name}-error`;

  // Атрибуты связи поля с ошибкой и ref для фокуса на первой ошибке после отправки.
  const field = (name: Field<S>) => ({
    id: name as string,
    "aria-invalid": errors[name] !== undefined ? (true as const) : undefined,
    "aria-describedby": errors[name] !== undefined ? errorId(name) : undefined,
    ref: (el: HTMLElement | null) => {
      if (el) elements.current.set(name, el);
      else elements.current.delete(name);
    },
  });

  return {
    values,
    errors,
    submitting,
    setValue,
    blur,
    handleSubmit,
    applyServerErrors,
    field,
    errorId,
  };
}
