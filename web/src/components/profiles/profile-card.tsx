"use client";

import { Copy, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import type { DeletionRequest, Profile } from "@/lib/api/gen/model";
import { useCopyText } from "@/lib/clipboard";
import { useFormat, useMessages } from "@/lib/i18n/client";
import { DeletionStatusBadge } from "./deletion-status-badge";

type ProfileCardProps = {
  profile: Profile;
  timezone: string;
  /** Последний запрос на удаление по этому person. */
  deletion?: DeletionRequest | undefined;
  onDelete: () => void;
};

export function isDeletionActive(deletion: DeletionRequest | undefined): boolean {
  return deletion?.status === "pending" || deletion?.status === "running";
}

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex gap-1.5">
      <dt className="text-fg-muted">{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

// Сводка по person (14-ux-ui-spec.md S-09): person_key с копированием, даты в TZ приложения,
// статус удаления. Пока запрос в очереди или выполняется, «Удалить данные» недоступна.
export function ProfileCard({ profile, timezone, deletion, onDelete }: ProfileCardProps) {
  const copy = useCopyText();
  const t = useMessages().profiles.card;
  const f = useFormat();
  return (
    <Card className="gap-4 px-6">
      <div className="flex items-start gap-3">
        <h2 className="min-w-0 font-mono text-heading-3 break-all">{profile.person_key}</h2>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t.copyKey}
          onClick={() => void copy(profile.person_key)}
        >
          <Copy aria-hidden="true" />
        </Button>
        <Button
          variant="outline"
          className="ml-auto text-destructive"
          disabled={isDeletionActive(deletion)}
          onClick={onDelete}
        >
          <Trash2 aria-hidden="true" />
          {t.deleteData}
        </Button>
      </div>
      <dl className="flex flex-wrap gap-x-6 gap-y-1 text-body-md">
        {profile.user_id !== null ? (
          <Fact label="user_id:">
            <span className="font-mono text-code break-all">{profile.user_id}</span>
          </Fact>
        ) : null}
        <Fact label={t.firstSeen}>{f.formatDate(profile.first_seen, timezone)}</Fact>
        <Fact label={t.lastSeen}>{f.formatDateTime(profile.last_seen, timezone)}</Fact>
      </dl>
      {/* biome-ignore lint/a11y/useSemanticElements: подпись и бейдж статуса, а не набор полей формы */}
      <div
        role="group"
        aria-label={t.deletionStatus}
        className="flex items-center gap-2 text-body-md"
      >
        <span className="text-fg-muted">{t.deletionStatusLabel}</span>{" "}
        {deletion ? (
          <>
            <DeletionStatusBadge status={deletion.status} />{" "}
            {deletion.status === "done" && deletion.completed_at ? (
              <span className="tabular-nums">
                {f.formatDateTime(deletion.completed_at, timezone)}
              </span>
            ) : null}
            {deletion.status === "failed" && deletion.error ? (
              <span className="text-destructive">{deletion.error}</span>
            ) : null}
          </>
        ) : (
          <span>—</span>
        )}
      </div>
    </Card>
  );
}
