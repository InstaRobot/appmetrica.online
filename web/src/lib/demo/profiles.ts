// Профили демо-режима (S-08, S-09): несколько описанных вручную людей на приложение (их ID можно
// показать подсказкой в поиске) и детерминированный пул «фоновых» людей, из которого берутся события
// realtime. Сессии лежат на абсолютной сетке дней, поэтому курсор before/next_before стабилен.
import type {
  Environment,
  Profile,
  ProfileInstall,
  ProfileSearchResult,
  ProfileSession,
  ProfileSummary,
} from "@/lib/api/gen/model";
import { addDays, todayIn } from "@/lib/period";
import { DEMO_APP_IDS } from "./apps";
import { type DemoApp, type InstallAttrs, sampleAttrs } from "./model";
import { hexFrom, pickWeighted, rand, seeded, uuidFrom, type Weighted } from "./random";
import { eachDay, HOUR_MS, isoUtc, maxDay, minDay, zonedMidnight } from "./time";

export type DemoProfileExample = {
  appId: string;
  personKey: string;
  userId: string | null;
  installId: string;
  label: string;
};

type Spec = {
  appId: string;
  userId: string | null;
  label: string;
  installs: readonly { id: string; attrs: Partial<InstallAttrs> }[];
  since: string;
  activity: number;
  perDay: number;
  devShare?: number;
};

const CURATED: readonly Spec[] = [
  {
    appId: DEMO_APP_IDS.notesIos,
    userId: "a3f9c2d1-77b4-4e0a-9c1e-2b5d8f6e1a90",
    label: "Активный пользователь: iPhone и iPad",
    installs: [
      {
        id: "6f1c2a9e-3b7d-4c55-8a2e-0d9b1f7e4c21",
        attrs: {
          os_name: "iOS",
          os_version: "26.0.1",
          device_model: "iPhone17,3",
          device_type: "phone",
          app_version: "3.4.0",
          country: "RU",
          city: "Moscow",
          locale: "ru-RU",
        },
      },
      {
        id: "9b2e4d6f-8a1c-4e3b-a5d7-c9e1f3a5b7d9",
        attrs: {
          os_name: "iPadOS",
          os_version: "26.0.1",
          device_model: "iPad14,3",
          device_type: "tablet",
          app_version: "3.4.0",
          country: "RU",
          city: "Moscow",
          locale: "ru-RU",
        },
      },
    ],
    since: "2026-04-20",
    activity: 0.92,
    perDay: 2.6,
  },
  {
    appId: DEMO_APP_IDS.notesIos,
    userId: "user_48213",
    label: "Пользователь с одним устройством",
    installs: [
      {
        id: "3d5f7a9b-1c3e-4a5b-8c7d-9e1f2a3b4c5d",
        attrs: {
          os_name: "iOS",
          os_version: "18.6.2",
          device_model: "iPhone15,4",
          device_type: "phone",
          app_version: "3.3.2",
          country: "RU",
          city: "Saint Petersburg",
          locale: "ru-RU",
        },
      },
    ],
    since: "2026-08-03",
    activity: 0.35,
    perDay: 1.4,
  },
  {
    appId: DEMO_APP_IDS.notesIos,
    userId: null,
    label: "Анонимная установка без user_id",
    installs: [
      {
        id: "c4e6a8b0-2d4f-4a6b-9c8d-0e2f4a6b8c0d",
        attrs: {
          os_name: "iOS",
          os_version: "17.7.2",
          device_model: "iPhone12,1",
          device_type: "phone",
          app_version: "3.3.1",
          country: "KZ",
          city: "Almaty",
          locale: "ru-KZ",
        },
      },
    ],
    since: "2026-09-21",
    activity: 0.4,
    perDay: 1.3,
  },
  {
    appId: DEMO_APP_IDS.notesIos,
    userId: "qa-tester-01",
    label: "Тестировщик: сессии development",
    installs: [
      {
        id: "e7a9c1e3-5b7d-4f9a-b1c3-d5e7f9a1b3c5",
        attrs: {
          os_name: "iOS",
          os_version: "26.0.1",
          device_model: "iPhone16,2",
          device_type: "phone",
          app_version: "3.4.0",
          country: "RU",
          city: "Moscow",
          locale: "ru-RU",
        },
      },
    ],
    since: "2026-06-15",
    activity: 0.7,
    perDay: 3,
    devShare: 0.85,
  },
  {
    appId: DEMO_APP_IDS.notesAndroid,
    userId: "8c1d2e3f-4a5b-4c6d-8e7f-9a0b1c2d3e4f",
    label: "Активный пользователь",
    installs: [
      {
        id: "1a2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
        attrs: {
          os_name: "Android",
          os_version: "15",
          device_model: "SM-S928B",
          device_type: "phone",
          app_version: "3.4.0",
          country: "RU",
          city: "Kazan",
          locale: "ru-RU",
        },
      },
    ],
    since: "2026-05-11",
    activity: 0.85,
    perDay: 2.4,
  },
  {
    appId: DEMO_APP_IDS.notesAndroid,
    userId: "user_10577",
    label: "Телефон и планшет одного пользователя",
    installs: [
      {
        id: "2b3c4d5e-6f7a-4b8c-9d0e-1f2a3b4c5d6e",
        attrs: {
          os_name: "Android",
          os_version: "16",
          device_model: "Pixel 8",
          device_type: "phone",
          app_version: "3.4.0",
          country: "RU",
          city: "Novosibirsk",
          locale: "ru-RU",
        },
      },
      {
        id: "3c4d5e6f-7a8b-4c9d-a0e1-2f3a4b5c6d7e",
        attrs: {
          os_name: "Android",
          os_version: "14",
          device_model: "SM-X710",
          device_type: "tablet",
          app_version: "3.3.2",
          country: "RU",
          city: "Novosibirsk",
          locale: "ru-RU",
        },
      },
    ],
    since: "2026-07-01",
    activity: 0.6,
    perDay: 1.8,
  },
  {
    appId: DEMO_APP_IDS.notesAndroid,
    userId: null,
    label: "Анонимная установка без user_id",
    installs: [
      {
        id: "4d5e6f7a-8b9c-4dae-b1f2-3a4b5c6d7e8f",
        attrs: {
          os_name: "Android",
          os_version: "13",
          device_model: "23129RAA4G",
          device_type: "phone",
          app_version: "3.3.1",
          country: "UZ",
          city: "Tashkent",
          locale: "ru-UZ",
        },
      },
    ],
    since: "2026-09-02",
    activity: 0.3,
    perDay: 1.5,
  },
  {
    appId: DEMO_APP_IDS.notesAndroid,
    userId: "org/42",
    label: "user_id со слешем",
    installs: [
      {
        id: "5e6f7a8b-9cad-4ebf-82a3-4b5c6d7e8f90",
        attrs: {
          os_name: "Android",
          os_version: "15",
          device_model: "SM-A556E",
          device_type: "phone",
          app_version: "3.4.0",
          country: "BY",
          city: "Minsk",
          locale: "ru-BY",
        },
      },
    ],
    since: "2026-08-18",
    activity: 0.5,
    perDay: 1.7,
  },
  {
    appId: DEMO_APP_IDS.site,
    userId: "site-5521",
    label: "Постоянный посетитель с компьютера",
    installs: [
      {
        id: "6f7a8b9c-adbe-4fc0-93b4-5c6d7e8f90a1",
        attrs: {
          os_name: "Windows",
          os_version: "11",
          device_model: "",
          device_type: "desktop",
          app_version: "2026.10.2",
          country: "RU",
          city: "Moscow",
          locale: "ru-RU",
        },
      },
    ],
    since: "2026-06-01",
    activity: 0.75,
    perDay: 1.9,
  },
  {
    appId: DEMO_APP_IDS.site,
    userId: null,
    label: "Анонимный посетитель с iPhone",
    installs: [
      {
        id: "7a8b9cad-becf-4d01-a4c5-6d7e8f90a1b2",
        attrs: {
          os_name: "iOS",
          os_version: "26.0.1",
          device_model: "iPhone",
          device_type: "phone",
          app_version: "2026.10.2",
          country: "RU",
          city: "Saint Petersburg",
          locale: "ru-RU",
        },
      },
    ],
    since: "2026-10-02",
    activity: 0.35,
    perDay: 1.3,
  },
  {
    appId: DEMO_APP_IDS.site,
    userId: "site-9034",
    label: "Пользователь с двух устройств",
    installs: [
      {
        id: "8b9cadbe-cfd0-4e12-b5d6-7e8f90a1b2c3",
        attrs: {
          os_name: "macOS",
          os_version: "15.6.1",
          device_model: "Macintosh",
          device_type: "desktop",
          app_version: "2026.10.2",
          country: "RU",
          city: "Yekaterinburg",
          locale: "ru-RU",
        },
      },
      {
        id: "9cadbecf-d0e1-4f23-86e7-8f90a1b2c3d4",
        attrs: {
          os_name: "Android",
          os_version: "15",
          device_model: "SM-A556E",
          device_type: "phone",
          app_version: "2026.10.1",
          country: "RU",
          city: "Yekaterinburg",
          locale: "ru-RU",
        },
      },
    ],
    since: "2026-07-20",
    activity: 0.5,
    perDay: 1.6,
  },
  {
    appId: DEMO_APP_IDS.bot,
    userId: "tg_5f2c81e9a7",
    label: "Ежедневный пользователь бота",
    installs: [
      {
        id: "adbecfd0-e1f2-4a34-97f8-90a1b2c3d4e5",
        attrs: {
          os_name: "Android",
          device_type: "phone",
          app_version: "1.8.0",
          country: "RU",
          city: "Moscow",
          locale: "ru",
        },
      },
    ],
    since: "2026-03-02",
    activity: 0.8,
    perDay: 3,
  },
  {
    appId: DEMO_APP_IDS.bot,
    userId: "tg_0b7d33c4f1",
    label: "Пользователь из Казахстана",
    installs: [
      {
        id: "becfd0e1-f2a3-4b45-a809-a1b2c3d4e5f6",
        attrs: {
          os_name: "iOS",
          device_type: "phone",
          app_version: "1.8.0",
          country: "KZ",
          city: "Almaty",
          locale: "ru",
        },
      },
    ],
    since: "2026-08-25",
    activity: 0.45,
    perDay: 2,
  },
  {
    appId: DEMO_APP_IDS.bot,
    userId: "tg_9e41a6b2d8",
    label: "Telegram Desktop на старой версии",
    installs: [
      {
        id: "cfd0e1f2-a3b4-4c56-b91a-b2c3d4e5f6a7",
        attrs: {
          os_name: "Windows",
          device_type: "desktop",
          app_version: "1.7.3",
          country: "RU",
          city: "Kazan",
          locale: "ru",
        },
      },
    ],
    since: "2026-09-28",
    activity: 0.6,
    perDay: 2.2,
  },
  {
    appId: DEMO_APP_IDS.oldGame,
    userId: "player_771",
    label: "Игрок до архивации",
    installs: [
      {
        id: "d0e1f2a3-b4c5-4d67-8a2b-c3d4e5f6a7b8",
        attrs: {
          os_name: "iOS",
          os_version: "17.7.2",
          device_model: "iPhone14,5",
          device_type: "phone",
          app_version: "1.12.0",
          country: "RU",
          city: "Moscow",
          locale: "ru-RU",
        },
      },
    ],
    since: "2025-11-01",
    activity: 0.7,
    perDay: 2.5,
  },
  {
    appId: DEMO_APP_IDS.oldGame,
    userId: null,
    label: "Анонимный игрок на iPad",
    installs: [
      {
        id: "e1f2a3b4-c5d6-4e78-9b3c-d4e5f6a7b8c9",
        attrs: {
          os_name: "iPadOS",
          os_version: "18.6.2",
          device_model: "iPad13,18",
          device_type: "tablet",
          app_version: "1.11.3",
          country: "RU",
          city: "Samara",
          locale: "ru-RU",
        },
      },
    ],
    since: "2026-02-14",
    activity: 0.4,
    perDay: 1.6,
  },
];

function personKeyOf(spec: Pick<Spec, "userId" | "installs">): string {
  return spec.userId !== null ? `u:${spec.userId}` : `i:${spec.installs[0]?.id ?? ""}`;
}

// Примеры для подсказки в поиске профилей: install_id и user_id, которые точно находятся.
export const DEMO_PROFILES: readonly DemoProfileExample[] = CURATED.map((spec) => ({
  appId: spec.appId,
  personKey: personKeyOf(spec),
  userId: spec.userId,
  installId: spec.installs[0]?.id ?? "",
  label: spec.label,
}));

export function demoProfileExamples(appId: string): DemoProfileExample[] {
  return DEMO_PROFILES.filter((p) => p.appId === appId);
}

// --- люди приложения ---

export type DemoInstall = InstallAttrs & { installId: string; previousVersion: string };

export type DemoPerson = {
  key: string;
  userId: string | null;
  installs: DemoInstall[];
  since: string;
  activity: number;
  perDay: number;
  devShare: number;
  curated: boolean;
};

export const POOL_SIZE = 300;
// Дата, от которой отсчитаны начала пула: история профилей не зависит от «сейчас».
const POOL_ANCHOR = "2026-10-14";

function previousVersion(demo: DemoApp, current: string): string {
  const spec = demo.model.dims.app_version;
  const versions = "weights" in spec ? spec.weights.map(([v]) => v) : [];
  const i = versions.indexOf(current);
  return versions[i + 1] ?? current;
}

function install(demo: DemoApp, id: string, overrides: Partial<InstallAttrs>): DemoInstall {
  const attrs = { ...sampleAttrs(demo.model, seeded(demo.app.id, "install", id)), ...overrides };
  return { ...attrs, installId: id, previousVersion: previousVersion(demo, attrs.app_version) };
}

function poolUserId(demo: DemoApp, i: number): string {
  const hex = hexFrom(10, demo.app.id, "pool-user", i);
  switch (demo.app.platform) {
    case "web":
      return `site-${hex.slice(0, 6)}`;
    case "bot":
      return `tg_${hex}`;
    default:
      return `usr_${hex}`;
  }
}

function poolPerson(demo: DemoApp, i: number): DemoPerson {
  const id = demo.app.id;
  const r = seeded(id, "pool", i);
  const linked = demo.app.platform === "bot" || r() < 0.68;
  const twoDevices = linked && r() < 0.15;
  const installs = [install(demo, uuidFrom(id, "pool", i, 0), {})];
  if (twoDevices) installs.push(install(demo, uuidFrom(id, "pool", i, 1), {}));
  const anchor = demo.lastDay ?? POOL_ANCHOR;
  const since = maxDay(demo.firstDay, addDays(anchor, -(3 + Math.floor(r() * 200))));
  const userId = linked ? poolUserId(demo, i) : null;
  return {
    key: userId !== null ? `u:${userId}` : `i:${installs[0]?.installId ?? ""}`,
    userId,
    installs,
    since,
    activity: 0.25 + r() * 0.6,
    perDay: 1.2 + r() * 2,
    devShare: 0,
    curated: false,
  };
}

type Directory = {
  persons: DemoPerson[];
  byKey: Map<string, DemoPerson>;
  byInstall: Map<string, DemoPerson>;
  byUser: Map<string, DemoPerson>;
};

const directories = new Map<string, Directory>();

function directory(demo: DemoApp): Directory {
  const cached = directories.get(demo.app.id);
  if (cached) return cached;
  const persons: DemoPerson[] = CURATED.filter((s) => s.appId === demo.app.id).map((s) => ({
    key: personKeyOf(s),
    userId: s.userId,
    installs: s.installs.map((i) => install(demo, i.id, i.attrs)),
    since: s.since,
    activity: s.activity,
    perDay: s.perDay,
    devShare: s.devShare ?? 0,
    curated: true,
  }));
  for (let i = 0; i < POOL_SIZE; i++) persons.push(poolPerson(demo, i));
  const dir: Directory = { persons, byKey: new Map(), byInstall: new Map(), byUser: new Map() };
  for (const p of persons) {
    dir.byKey.set(p.key, p);
    if (p.userId !== null) dir.byUser.set(p.userId, p);
    for (const i of p.installs) dir.byInstall.set(i.installId, p);
  }
  directories.set(demo.app.id, dir);
  return dir;
}

export function appPersons(demo: DemoApp): readonly DemoPerson[] {
  return directory(demo).persons;
}

// --- сессии ---

type Session = {
  id: string;
  install: DemoInstall;
  start: number;
  end: number | null;
  durationMs: number | null;
  version: string;
  environment: Environment;
};

const hourWeights = new Map<DemoApp, Weighted<number>>();

function hoursOf(demo: DemoApp): Weighted<number> {
  let w = hourWeights.get(demo);
  if (!w) {
    w = demo.model.hourly.map((weight, hour) => [hour, weight] as const);
    hourWeights.set(demo, w);
  }
  return w;
}

// sessionsOf — сессии человека до now, новые первыми. Первый день истории всегда активен, поэтому
// у каждого человека есть хотя бы одна сессия.
function sessionsOf(demo: DemoApp, person: DemoPerson, now: number): Session[] {
  const tz = demo.app.timezone;
  const id = demo.app.id;
  const last = minDay(todayIn(tz, new Date(now)), demo.lastDay ?? "9999-12-31");
  const switchDay = addDays(
    demo.lastDay ?? POOL_ANCHOR,
    -(10 + Math.floor(rand(id, person.key, "upd") * 30)),
  );
  const out: Session[] = [];
  for (const day of eachDay(person.since, last)) {
    const r = seeded(id, person.key, day);
    const active = r() < person.activity || day === person.since;
    const count = 1 + Math.floor(r() * (2 * person.perDay - 1));
    if (!active) continue;
    const midnight = zonedMidnight(day, tz);
    for (let j = 0; j < count; j++) {
      const hour = pickWeighted(hoursOf(demo), r());
      const start = midnight + hour * HOUR_MS + Math.floor(r() * HOUR_MS);
      const inst = person.installs[r() < 0.75 ? 0 : Math.floor(r() * person.installs.length)];
      const duration = Math.round(
        Math.min(
          3 * HOUR_MS,
          Math.max(4_000, demo.model.avgDurationSec * 1000 * Math.exp((r() - 0.5) * 1.6)),
        ),
      );
      const environment: Environment = r() < person.devShare ? "development" : "production";
      if (!inst || start > now) continue;
      const end = start + duration;
      out.push({
        id: uuidFrom(id, person.key, day, j),
        install: inst,
        start,
        end: end <= now ? end : null,
        durationMs: end <= now ? duration : null,
        version: day < switchDay ? inst.previousVersion : inst.app_version,
        environment,
      });
    }
  }
  return out.sort((a, b) => b.start - a.start || a.id.localeCompare(b.id));
}

function lastEvent(s: Session): number {
  return s.end ?? s.start;
}

type Seen = { first: number; last: number };

function seen(sessions: readonly Session[]): Seen {
  let first = Number.POSITIVE_INFINITY;
  let last = 0;
  for (const s of sessions) {
    first = Math.min(first, s.start);
    last = Math.max(last, lastEvent(s));
  }
  return { first, last };
}

function installsOf(sessions: readonly Session[]): ProfileInstall[] {
  const byInstall = new Map<string, Session[]>();
  for (const s of sessions) {
    const list = byInstall.get(s.install.installId) ?? [];
    list.push(s);
    byInstall.set(s.install.installId, list);
  }
  return [...byInstall.values()].flatMap((list) => {
    const latest = list[0];
    if (!latest) return [];
    const i = latest.install;
    const span = seen(list);
    return [
      {
        install_id: i.installId,
        platform: i.platform,
        os_name: i.os_name,
        os_version: i.os_version,
        app_version: latest.version,
        device_model: i.device_model,
        device_type: i.device_type,
        locale: i.locale,
        country: i.country,
        city: i.city,
        first_seen: isoUtc(span.first),
        last_seen: isoUtc(span.last),
      },
    ];
  });
}

function summaryOf(
  demo: DemoApp,
  person: DemoPerson,
  matchedBy: ProfileSummary["matched_by"],
  now: number,
): ProfileSummary | null {
  const sessions = sessionsOf(demo, person, now);
  if (sessions.length === 0) return null;
  const span = seen(sessions);
  return {
    person_key: person.key,
    user_id: person.userId,
    matched_by: matchedBy,
    installs_count: new Set(sessions.map((s) => s.install.installId)).size,
    first_seen: isoUtc(span.first),
    last_seen: isoUtc(span.last),
  };
}

// searchProfiles — точное совпадение q с install_id (профиль его человека) или с user_id.
export function searchProfiles(demo: DemoApp, q: string, now: number): ProfileSearchResult {
  const dir = directory(demo);
  const items: ProfileSummary[] = [];
  const byInstall = dir.byInstall.get(q.toLowerCase());
  if (byInstall) {
    const s = summaryOf(demo, byInstall, "install_id", now);
    if (s) items.push(s);
  }
  const byUser = dir.byUser.get(q);
  if (byUser && byUser !== byInstall) {
    const s = summaryOf(demo, byUser, "user_id", now);
    if (s) items.push(s);
  }
  return { items };
}

// getProfile — профиль по person_key; null — у человека нет событий (404 profile_not_found).
export function getProfile(
  demo: DemoApp,
  personKey: string,
  page: { limit: number; before: number | null },
  now: number,
): Profile | null {
  const person = directory(demo).byKey.get(personKey);
  if (!person) return null;
  const sessions = sessionsOf(demo, person, now);
  if (sessions.length === 0) return null;
  const span = seen(sessions);
  const { before } = page;
  const pageSessions = sessions
    .filter((s) => before === null || s.start < before)
    .slice(0, page.limit);
  const lastOfPage = pageSessions.at(-1);
  return {
    person_key: person.key,
    user_id: person.userId,
    first_seen: isoUtc(span.first),
    last_seen: isoUtc(span.last),
    installs: installsOf(sessions),
    sessions: pageSessions.map(
      (s): ProfileSession => ({
        session_id: s.id,
        install_id: s.install.installId,
        started_at: isoUtc(s.start),
        ended_at: s.end === null ? null : isoUtc(s.end),
        duration_ms: s.durationMs,
        app_version: s.version,
        platform: s.install.platform,
        environment: s.environment,
        country: s.install.country,
        city: s.install.city,
      }),
    ),
    next_before: pageSessions.length === page.limit && lastOfPage ? isoUtc(lastOfPage.start) : null,
  };
}
