import { toast } from "sonner";

// copyText копирует текст и объявляет результат через sonner (aria-live): кнопки копирования
// CopyField, CodeBlock и «Код запроса» ErrorState.
export async function copyText(value: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(value);
    toast.success("Скопировано", { duration: 1500 });
  } catch {
    toast.error("Не удалось скопировать");
  }
}
