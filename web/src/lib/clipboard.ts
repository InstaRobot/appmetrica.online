import { useCallback } from "react";
import { useMessages } from "@/lib/i18n/client";
import { toast } from "@/lib/toast";

export type CopyTexts = { copied: string; failed: string };

// Русские тексты для вызовов copyText, ещё не переведённых на useCopyText; удаляются вместе с русскими
// обёртками lib/format.
const RU_TEXTS: CopyTexts = { copied: "Скопировано", failed: "Не удалось скопировать" };

// copyText копирует текст и объявляет результат через sonner (aria-live): кнопки копирования
// CopyField, CodeBlock и «Код запроса» ErrorState.
export async function copyText(value: string, texts: CopyTexts = RU_TEXTS): Promise<void> {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(texts.copied, { duration: 1500 });
  } catch {
    toast.error(texts.failed);
  }
}

// copyText с текстами языка интерфейса (словарь common.copy).
export function useCopyText(): (value: string) => Promise<void> {
  const texts = useMessages().common.copy;
  return useCallback((value: string) => copyText(value, texts), [texts]);
}
