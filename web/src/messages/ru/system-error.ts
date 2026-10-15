import type { systemError as en } from "../en/system-error";

export const systemError: typeof en = {
  title: "Что-то пошло не так",
  text: "Произошла непредвиденная ошибка. Попробуйте обновить страницу.",
  retry: "Попробовать снова",
  toPortfolio: "В портфель",
  requestId: "Код запроса: {id}",
  copyRequestId: "Копировать код запроса",
  copied: "Скопировано",
  copyFailed: "Не удалось скопировать",
};
