"use client";

import { lazy, Suspense } from "react";

// sonner грузится отдельным чанком сразу после первой загрузки страницы (lib/toast.ts).
const MountedToaster = lazy(() =>
  import("./mounted-toaster").then((m) => ({ default: m.MountedToaster })),
);

export function LazyToaster() {
  return (
    <Suspense fallback={null}>
      <MountedToaster />
    </Suspense>
  );
}
