import type { Metadata } from "next";
import { JetBrains_Mono, Manrope } from "next/font/google";
import { LazyToaster } from "@/components/common/lazy-toaster";
import { TooltipProvider } from "@/components/ui/tooltip";
import { langInitScript } from "@/lib/i18n/lang-script";
import { DEFAULT_LOCALE } from "@/lib/i18n/locale";
import { themeInitScript } from "@/lib/theme";
import { cn } from "@/lib/utils";
import { getMessages } from "@/messages";
import "./globals.css";

// Шрифты по макету «Dashboard v2»: Manrope для интерфейса, JetBrains Mono для ID и кода.
const manrope = Manrope({
  subsets: ["latin", "cyrillic"],
  weight: ["400", "500", "600", "700", "800"],
  variable: "--font-manrope",
  display: "swap",
});
const jetbrainsMono = JetBrains_Mono({
  subsets: ["latin", "cyrillic"],
  weight: ["400", "500"],
  variable: "--font-jetbrains-mono",
  display: "swap",
});

// Корневой layout не знает языка запроса (cookie читается только под Suspense), поэтому описание — на
// языке по умолчанию; лендинг задаёт своё на языке адреса (app/[lang]/page.tsx).
export const metadata: Metadata = {
  title: { default: "appmetrica.online", template: "%s · appmetrica.online" },
  description: getMessages(DEFAULT_LOCALE).landing.meta.description,
  // Внутренняя система одного владельца: поисковым роботам здесь делать нечего.
  robots: { index: false, follow: false },
};

// data-theme и lang выставляют inline-скрипты до первой отрисовки (без мигания), поэтому атрибуты <html>
// на клиенте отличаются от серверных — suppressHydrationWarning только для этого узла. Язык по умолчанию —
// английский; /ru… и выбор в cookie скрипт видит сам, точный язык сервера ставит HtmlLang.
export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" suppressHydrationWarning>
      <head>
        {/* biome-ignore lint/security/noDangerouslySetInnerHtml: статичная строка из lib/theme.ts, без пользовательских данных */}
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
        {/* biome-ignore lint/security/noDangerouslySetInnerHtml: статичная строка из lib/i18n/lang-script.ts, без пользовательских данных */}
        <script dangerouslySetInnerHTML={{ __html: langInitScript }} />
      </head>
      <body className={cn(manrope.variable, jetbrainsMono.variable, "font-sans antialiased")}>
        <TooltipProvider>{children}</TooltipProvider>
        <LazyToaster />
      </body>
    </html>
  );
}
