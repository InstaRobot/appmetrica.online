// Тексты ошибки страницы — часть пространства имён system. Отдельный модуль: корневые app/error.tsx и
// app/global-error.tsx стоят вне MessagesProvider и импортируют его обоих языков напрямую, поэтому он
// попадает в initial JS каждой страницы — держать его коротким.
export const systemError = {
  title: "Something went wrong",
  text: "An unexpected error occurred. Try refreshing the page.",
  retry: "Try again",
  toPortfolio: "Go to portfolio",
  requestId: "Request ID: {id}",
  copyRequestId: "Copy request ID",
  copied: "Copied",
  copyFailed: "Couldn't copy",
};
