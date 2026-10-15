import type { system as en } from "../en/system";
import { systemError } from "./system-error";

export const system: typeof en = {
  notFound: {
    title: "Страница не найдена",
    text: "Проверьте адрес или вернитесь в портфель.",
    action: "В портфель",
  },
  docsNotFound: {
    text: "Такой страницы в документации нет.",
    action: "К быстрому старту",
  },
  error: systemError,
};
