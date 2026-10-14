import type { Route } from "next";
import type { CreateDeletionRequestBody } from "@/lib/api/gen/model";

// person_key профиля (05-data-model.md, OQ-047): `u:<user_id>` для склеенного пользователя или
// `i:<install_id>` для установки без user_id. В URL S-09 и в пути getProfile — один сегмент
// encodeURIComponent, поэтому `/`, пробел и `%` в user_id переживают round-trip.

export type PersonKind = "user" | "install";
export type PersonKey = { kind: PersonKind; value: string };

const PREFIX: Record<PersonKind, string> = { user: "u:", install: "i:" };

export function formatPersonKey(key: PersonKey): string {
  return `${PREFIX[key.kind]}${key.value}`;
}

export function parsePersonKey(key: string): PersonKey | null {
  const kind = key.startsWith("u:") ? "user" : key.startsWith("i:") ? "install" : null;
  const value = key.slice(2);
  return kind && value ? { kind, value } : null;
}

export function encodePersonKey(key: PersonKey | string): string {
  return encodeURIComponent(typeof key === "string" ? key : formatPersonKey(key));
}

// Сегмент пути S-09. Закодированный ключ всегда начинается с `u%3A`/`i%3A` (двоеточие кодируется),
// уже раскодированный роутером — с `u:`/`i:`, поэтому повторного декодирования `%` в user_id нет.
export function decodePersonKey(segment: string): PersonKey | null {
  if (/^[ui]%3[Aa]/.test(segment)) {
    try {
      return parsePersonKey(decodeURIComponent(segment));
    } catch {
      return null;
    }
  }
  return parsePersonKey(segment);
}

export function profileHref(appId: string, key: PersonKey | string): Route {
  return `/apps/${appId}/profiles/${encodePersonKey(key)}` as Route;
}

// Тело createDeletionRequest: `u:` → user, `i:` → install (14-ux-ui-spec.md S-09).
export function deletionSubject(key: PersonKey): CreateDeletionRequestBody {
  return { subject_type: key.kind, subject_value: key.value };
}

// Запрос поиска S-08: обрезается по краям, префикс `u:`/`i:` срезается и задаёт тип совпадения.
export function parseSearchQuery(q: string): { value: string; kind: PersonKind | null } {
  const trimmed = q.trim();
  for (const kind of ["user", "install"] as const) {
    if (trimmed.startsWith(PREFIX[kind])) {
      return { value: trimmed.slice(2).trim(), kind };
    }
  }
  return { value: trimmed, kind: null };
}
