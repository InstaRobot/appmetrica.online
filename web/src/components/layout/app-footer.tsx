import type { Route } from "next";
import Link from "next/link";

// Документация (S-13) появляется в T-047; до этого typedRoutes не знает маршрут "/docs".
const DOCS = "/docs" as Route;

// Футер дашборда: атрибуция DB-IP обязательна по лицензии CC BY 4.0 базы IP-to-City Lite.
export function AppFooter() {
  return (
    <footer className="mx-auto flex max-w-[1600px] items-center justify-between gap-4 px-8 pb-8 text-caption text-fg-muted">
      <Link href={DOCS} className="hover:text-foreground hover:underline">
        Документация
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
