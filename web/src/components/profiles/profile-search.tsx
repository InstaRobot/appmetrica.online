"use client";

import { CircleAlert, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useZodForm } from "@/lib/forms/use-zod-form";
import { useMessages } from "@/lib/i18n/client";

type ProfileSearchProps = {
  defaultValue?: string;
  onSearch: (q: string) => void;
  pending?: boolean;
};

// F-8 (14-ux-ui-spec.md §7.1): значение обрезается по краям, 1–200 символов.
const schema = () => import("@/lib/forms/schemas").then((m) => m.profileSearchSchema);
const HINT_ID = "profile-search-hint";

// Поиск профиля по точному install_id или user_id (14-ux-ui-spec.md S-08, FR-016): Enter или «Найти».
export function ProfileSearch({
  defaultValue = "",
  onSearch,
  pending = false,
}: ProfileSearchProps) {
  const t = useMessages().profiles.search;
  const form = useZodForm(schema, {
    defaultValues: { q: defaultValue },
    messages: { q: { too_small: t.required, too_big: t.tooLong } },
    onSubmit: ({ q }) => onSearch(q),
  });
  const field = form.field("q");

  return (
    <search>
      <form noValidate onSubmit={form.handleSubmit} className="flex flex-col gap-1.5">
        <Label htmlFor={field.id} className="sr-only">
          {t.label}
        </Label>
        <div className="flex gap-2">
          <Input
            {...field}
            type="search"
            autoComplete="off"
            spellCheck={false}
            placeholder={t.label}
            className="font-mono text-code"
            aria-describedby={field["aria-describedby"] ?? HINT_ID}
            value={form.values.q}
            onChange={(e) => form.setValue("q", e.target.value)}
          />
          <Button type="submit" disabled={pending}>
            <Search aria-hidden="true" />
            {t.submit}
          </Button>
        </div>
        {form.errors.q ? (
          <p id={form.errorId("q")} className="flex items-center gap-1 text-caption text-negative">
            <CircleAlert aria-hidden="true" className="size-3.5 shrink-0" />
            {form.errors.q}
          </p>
        ) : (
          <p id={HINT_ID} className="text-caption text-fg-muted">
            {t.hint}
          </p>
        )}
      </form>
    </search>
  );
}
