import Link from "next/link";
import type { Locale } from "@/lib/i18n/locale";
import { publicHref } from "@/lib/i18n/routes";
import { getMessages } from "@/messages";

// Футер дашборда: атрибуция DB-IP обязательна по лицензии CC BY 4.0 базы IP-to-City Lite. Серверный
// компонент: язык передаёт layout панели, словарь в клиентский JS не попадает.
export function AppFooter({ locale }: { locale: Locale }) {
  const m = getMessages(locale).layout.footer;
  return (
    <footer className="mx-auto flex w-full max-w-[1600px] flex-col items-start gap-2 px-4 pb-8 text-caption text-fg-muted md:flex-row md:items-center md:justify-between md:gap-4 md:px-8">
      <Link href={publicHref(locale, "/docs")} className="hover:text-foreground hover:underline">
        {m.docs}
      </Link>
      <p>
        <a href="https://db-ip.com" className="text-fg-link hover:underline">
          IP Geolocation by DB-IP
        </a>{" "}
        (CC BY 4.0)
      </p>
    </footer>
  );
}
