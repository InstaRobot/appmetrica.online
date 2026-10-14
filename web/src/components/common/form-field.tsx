import { CircleAlert } from "lucide-react";

// Ошибка поля формы (14-ux-ui-spec.md §7.2): под полем, связана с ним через aria-describedby.
export function FieldError({ id, message }: { id: string; message: string | undefined }) {
  if (!message) return null;
  return (
    <p id={id} className="flex items-center gap-1 text-caption text-negative">
      <CircleAlert aria-hidden="true" className="size-3.5 shrink-0" />
      {message}
    </p>
  );
}

// Звёздочка обязательного поля: обязательность объявляет aria-required самого поля.
export function Required() {
  return (
    <span aria-hidden="true" className="text-negative">
      *
    </span>
  );
}
