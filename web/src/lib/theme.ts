// Светлая и тёмная темы (решение владельца по макету «Dashboard v2», 14-ux-ui-spec.md §2.2).
// Тема задаётся атрибутом <html data-theme> (+ класс `dark` для Fumadocs и `dark:`-классов shadcn).
// Выбор владельца хранится в localStorage; без сохранённого выбора — системная тема.
export type Theme = "light" | "dark";

export const THEME_STORAGE_KEY = "am-theme";

// Выполняется в <head> до первой отрисовки, поэтому самодостаточен (без импортов) и не бросает.
export const themeInitScript = `(function(){var t;try{t=localStorage.getItem("${THEME_STORAGE_KEY}")}catch(e){}if(t!=="light"&&t!=="dark"){t=window.matchMedia&&window.matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light"}var r=document.documentElement;r.dataset.theme=t;r.classList.toggle("dark",t==="dark");r.style.colorScheme=t})();`;

export function readTheme(): Theme {
  return document.documentElement.dataset.theme === "dark" ? "dark" : "light";
}

export function applyTheme(theme: Theme): void {
  const root = document.documentElement;
  root.dataset.theme = theme;
  root.classList.toggle("dark", theme === "dark");
  root.style.colorScheme = theme;
  try {
    localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch {
    // Приватный режим / запрет хранилища: тема применится до перезагрузки страницы.
  }
}
