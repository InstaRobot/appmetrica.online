import type { Metadata } from "next";
import { getRequestLocale } from "@/lib/i18n/server";
import { getMessages } from "@/messages";
import { NewAppForm, NewAppHeader } from "./create-app-form";

export async function generateMetadata(): Promise<Metadata> {
  return { title: getMessages(await getRequestLocale()).apps.create.title };
}

// S-04 `/apps/new`: форма создания приложения шириной 560 px под заголовком.
export default function NewAppPage() {
  return (
    <>
      <NewAppHeader />
      <NewAppForm />
    </>
  );
}
