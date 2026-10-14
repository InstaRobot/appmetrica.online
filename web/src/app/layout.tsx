import type { Metadata } from "next";
import { JetBrains_Mono, Manrope } from "next/font/google";
import { LazyToaster } from "@/components/common/lazy-toaster";
import { TooltipProvider } from "@/components/ui/tooltip";
import { themeInitScript } from "@/lib/theme";
import { cn } from "@/lib/utils";
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

export const metadata: Metadata = {
  title: { default: "appmetrica.online", template: "%s · appmetrica.online" },
  description: "Аналитика приложений без SDK",
  // Внутренняя система одного владельца: поисковым роботам здесь делать нечего.
  robots: { index: false, follow: false },
};

// data-theme выставляет inline-скрипт до первой отрисовки (без мигания), поэтому атрибуты <html>
// на клиенте отличаются от серверных — suppressHydrationWarning только для этого узла.
export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru" suppressHydrationWarning>
      <head>
        {/* biome-ignore lint/security/noDangerouslySetInnerHtml: статичная строка из lib/theme.ts, без пользовательских данных */}
        <script dangerouslySetInnerHTML={{ __html: themeInitScript }} />
      </head>
      <body className={cn(manrope.variable, jetbrainsMono.variable, "font-sans antialiased")}>
        <TooltipProvider>{children}</TooltipProvider>
        <LazyToaster />
      </body>
    </html>
  );
}
