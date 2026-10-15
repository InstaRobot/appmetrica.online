import { LOCALE_COOKIE } from "./locale";

// Ставит <html lang> до первой отрисовки (рядом со скриптом темы, без импортов): /ru… — русский, лендинг и
// документация без префикса — английский, панель — сохранённый выбор или английский.
export const langInitScript = `(function(){try{var p=location.pathname,l;if(/^\\/ru(\\/|$)/.test(p))l="ru";else if(p==="/"||/^\\/docs(\\/|$)/.test(p))l="en";else{var m=document.cookie.match(/(?:^|; )${LOCALE_COOKIE}=(en|ru)(?:;|$)/);l=m?m[1]:"en"}document.documentElement.lang=l}catch(e){}})();`;
