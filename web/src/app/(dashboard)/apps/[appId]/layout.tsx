import { Suspense } from "react";
import { AppShell, AppShellSkeleton } from "@/components/layout/app-shell";

// AppShell вкладок приложения: читает appId и ?unit на клиенте, поэтому стоит под Suspense.
export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <Suspense fallback={<AppShellSkeleton />}>
      <AppShell>{children}</AppShell>
    </Suspense>
  );
}
