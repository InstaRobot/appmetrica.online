import type { Metadata } from "next";
import { LandingPage } from "@/components/landing/landing-page";

export const metadata: Metadata = {
  title: { absolute: "appmetrica.online — аналитика приложений без SDK" },
};

// S-00 Лендинг на «/». Портфель владельца — на /portfolio (за логином).
export default function HomePage() {
  return <LandingPage />;
}
