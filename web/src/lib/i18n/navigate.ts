// Смена языка — полная загрузка страницы: новый словарь приходит с сервером целиком, <html lang> и данные
// панели строятся заново. Отдельный модуль — чтобы тесты подменяли навигацию браузера.
export function openPage(href: string): void {
  window.location.assign(href);
}

export function reloadPage(): void {
  window.location.reload();
}
