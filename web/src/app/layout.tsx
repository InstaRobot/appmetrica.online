import type { Metadata } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import { LazyToaster } from "@/components/common/lazy-toaster";
import { TooltipProvider } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import "./globals.css";

const inter = Inter({ subsets: ["latin", "cyrillic"], variable: "--font-inter", display: "swap" });
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

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="ru">
      <body className={cn(inter.variable, jetbrainsMono.variable, "font-sans antialiased")}>
        <TooltipProvider>{children}</TooltipProvider>
        <LazyToaster />
      </body>
    </html>
  );
}
