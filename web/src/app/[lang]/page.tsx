import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { LandingPage } from "@/components/landing/landing-page";
import { isLocale } from "@/lib/i18n/locale";
import { getMessages } from "@/messages";

export async function generateMetadata({ params }: PageProps<"/[lang]">): Promise<Metadata> {
  const { lang } = await params;
  if (!isLocale(lang)) return {};
  const m = getMessages(lang).landing.meta;
  return { title: { absolute: m.title }, description: m.description };
}

// S-00 Лендинг на «/» и «/ru». Портфель владельца — на /portfolio (за логином).
export default async function HomePage({ params }: PageProps<"/[lang]">) {
  const { lang } = await params;
  if (!isLocale(lang)) notFound();
  return <LandingPage locale={lang} />;
}
