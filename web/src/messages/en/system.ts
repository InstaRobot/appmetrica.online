import { systemError } from "./system-error";
import { systemToaster } from "./system-toaster";

// Системные страницы: 404 и ошибки (14-ux-ui-spec.md S-14). Тексты ошибки — в system-error.ts: их
// импортируют корневые границы ошибок вне MessagesProvider.
export const system = {
  notFound: {
    title: "Page not found",
    text: "Check the address or go back to the portfolio.",
    action: "Go to portfolio",
  },
  docsNotFound: {
    text: "There's no such page in the documentation.",
    action: "Go to quick start",
  },
  error: systemError,
  toaster: systemToaster,
};
