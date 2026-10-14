import type { Metadata } from "next";
import { PageHeader } from "@/components/layout/page-header";
import { NewAppForm } from "./create-app-form";

export const metadata: Metadata = { title: "Новое приложение" };

// S-04 `/apps/new`: форма создания приложения шириной 560 px под заголовком.
export default function NewAppPage() {
  return (
    <>
      <PageHeader title="Новое приложение" />
      <NewAppForm />
    </>
  );
}
