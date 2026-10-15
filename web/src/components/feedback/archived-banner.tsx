"use client";

import { Archive } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { useMessages } from "@/lib/i18n/client";

// Баннер AppShell и S-11 для приложения со status = archived (14-ux-ui-spec.md S-10, S-11).
export function ArchivedBanner() {
  return (
    <Alert role="status" className="border-warning/40 text-warning">
      <Archive aria-hidden="true" />
      <AlertDescription className="text-warning">{useMessages().common.archived}</AlertDescription>
    </Alert>
  );
}
