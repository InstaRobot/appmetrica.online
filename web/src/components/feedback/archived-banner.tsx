import { Archive } from "lucide-react";
import { Alert, AlertDescription } from "@/components/ui/alert";

// Баннер AppShell и S-11 для приложения со status = archived (14-ux-ui-spec.md S-10, S-11).
export function ArchivedBanner() {
  return (
    <Alert role="status" className="border-warning/40 text-warning">
      <Archive aria-hidden="true" />
      <AlertDescription className="text-warning">
        Приложение в архиве: события отклоняются с кодом app_archived.
      </AlertDescription>
    </Alert>
  );
}
