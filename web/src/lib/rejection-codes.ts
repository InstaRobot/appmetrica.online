import type { Messages } from "@/messages";

// Тексты кодов отказов ingest для RejectionReasonsTable S-10 (14-ux-ui-spec.md §14.3) живут в словаре
// integration.rejectionCodes: семь RejectionCode событий и batch-коды. Предупреждения сервер не хранит,
// поэтому их там нет.

export type RejectionCodeTexts = Messages["integration"]["rejectionCodes"];
export type RejectionText = { description: string; hint: string | null };

// rejectionCode — описание и подсказка на языке словаря; неизвестный код (новее web) показывается как есть.
export function rejectionCode(code: string, texts: RejectionCodeTexts): RejectionText {
  return Object.hasOwn(texts, code)
    ? texts[code as keyof RejectionCodeTexts]
    : { description: code, hint: null };
}
