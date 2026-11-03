// Отчёт «События» демо-режима в форме ответов Read API (api.yaml: listEvents, getEventReport,
// getEventBreakdown) поверх model.ts. События считаются по дням из сессий приложения: дневное число
// событий — сессии × события на сессию × шум по ключу (app, env, name, day); недели, месяцы и часы
// режут те же дни, поэтому сумма ряда равна сводке. Каталог — EVENT_CATALOG: новое событие или свойство
// добавляется туда, а не в код расчёта.
import type {
  EventBreakdown,
  EventCount,
  EventList,
  EventPoint,
  EventReport,
  EventStat,
  EventValueCount,
  Granularity,
} from "@/lib/api/gen/model";
import { addDays } from "@/lib/period";
import { DEMO_APP_IDS } from "./apps";
import { dayStats, hourSessions, hourStats, periodStats, type Scope } from "./model";
import { jitter, roundRandom, splitTotal, type Weighted } from "./random";
import { buckets, previousPeriod } from "./reports";
import { eachDay, formatZoned, HOUR_MS, localTime, zonedMidnight } from "./time";

// Системные имена ingest: в отчёт «События» не входят (reports.systemEventNames в Go).
export const SYSTEM_EVENT_NAMES: readonly string[] = ["session_start", "session_end"];

// Пределы отчёта, как в backend/internal/reports/events.go.
export const EVENT_PROPERTY_KEYS_MAX = 50;
export const EVENT_NAME = /^[a-z][a-z0-9_]{0,63}$/;

type PropertySpec = {
  key: string;
  values: Weighted<string>;
  // Доля событий без ключа (0 — ключ есть всегда).
  missing: number;
};

type EventSpec = {
  name: string;
  perSession: number; // событий на сессию
  reach: number; // доля активных людей периода, сделавших событие
  property: PropertySpec;
};

// results: число 0…40 строкой, веса убывают — у значений есть хвост «остальные» при limit=20.
const SEARCH_RESULTS: Weighted<string> = Array.from({ length: 41 }, (_, i) => [String(i), 41 - i]);

const NOTE_SOURCES = [
  "keyboard",
  "voice",
  "share",
  "widget",
  "shortcut",
  "camera",
  "clipboard",
  "template",
  "import",
  "email",
  "web_clipper",
  "watch",
  "siri",
  "scan",
  "drag_drop",
  "dictation",
  "files",
  "messages",
  "safari",
  "spotlight",
  "notification",
  "handoff",
  "quick_note",
  "lock_screen",
  "api",
];

// События демо-приложений (имена — как в ленте realtime, live.ts).
export const EVENT_CATALOG: Readonly<Record<string, readonly EventSpec[]>> = {
  [DEMO_APP_IDS.notes]: [
    {
      name: "note_created",
      perSession: 1.6,
      reach: 0.62,
      property: {
        key: "source",
        values: NOTE_SOURCES.map((v, i): [string, number] => [v, 30 - i]),
        missing: 0.05,
      },
    },
    {
      name: "search_used",
      perSession: 0.7,
      reach: 0.34,
      property: { key: "results", values: SEARCH_RESULTS, missing: 0 },
    },
    {
      name: "note_shared",
      perSession: 0.35,
      reach: 0.18,
      property: {
        key: "target",
        values: [
          ["messages", 4],
          ["mail", 3],
          ["telegram", 2],
          ["copy_link", 1],
        ],
        missing: 0,
      },
    },
    {
      name: "paywall_shown",
      perSession: 0.2,
      reach: 0.14,
      property: {
        key: "place",
        values: [
          ["onboarding", 1],
          ["limit", 1],
          ["settings", 1],
        ],
        missing: 0,
      },
    },
    {
      name: "purchase",
      perSession: 0.03,
      reach: 0.025,
      property: {
        key: "plan",
        values: [
          ["monthly", 5],
          ["yearly", 3],
          ["lifetime", 1],
        ],
        missing: 0.1,
      },
    },
  ],
  [DEMO_APP_IDS.site]: [
    {
      name: "docs_opened",
      perSession: 0.9,
      reach: 0.4,
      property: {
        key: "section",
        values: [
          ["quickstart", 5],
          ["api", 3],
          ["integration", 2],
        ],
        missing: 0.03,
      },
    },
    {
      name: "signup_clicked",
      perSession: 0.08,
      reach: 0.06,
      property: {
        key: "plan",
        values: [
          ["free", 4],
          ["team", 1],
        ],
        missing: 0,
      },
    },
  ],
};

export function eventSpecs(scope: Scope): readonly EventSpec[] {
  return EVENT_CATALOG[scope.demo.app.id] ?? [];
}

// dayEvents — события имени за день.
function dayEvents(scope: Scope, spec: EventSpec, day: string): number {
  const sessions = dayStats(scope, day).sessions;
  const expected =
    sessions * spec.perSession * jitter(0.3, scope.demo.app.id, scope.env, spec.name, day, "ev");
  return roundRandom(expected, scope.demo.app.id, scope.env, spec.name, day, "ev");
}

type Totals = { events: number; people: number };

// people — не больше событий и не больше активных людей периода (reach — доля, шум по периоду).
function peopleOf(
  scope: Scope,
  spec: EventSpec,
  active: number,
  events: number,
  from: string,
  to: string,
): number {
  const raw = active * spec.reach * jitter(0.1, scope.demo.app.id, scope.env, spec.name, from, to);
  return Math.min(
    events,
    active,
    roundRandom(raw, scope.demo.app.id, scope.env, spec.name, from, to),
  );
}

function periodTotals(scope: Scope, spec: EventSpec, from: string, to: string): Totals {
  let events = 0;
  for (const day of eachDay(from, to)) events += dayEvents(scope, spec, day);
  if (events === 0) return { events: 0, people: 0 };
  return {
    events,
    people: peopleOf(scope, spec, periodStats(scope, from, to).active, events, from, to),
  };
}

// eventsList — имена с событиями текущего периода: events DESC, name ASC.
export function eventsList(scope: Scope, from: string, to: string, compare: boolean): EventList {
  const [pFrom, pTo] = previousPeriod(from, to);
  const events: EventStat[] = [];
  for (const spec of eventSpecs(scope)) {
    const cur = periodTotals(scope, spec, from, to);
    if (cur.events === 0) continue;
    const stat: EventStat = { name: spec.name, events: cur.events, people: cur.people };
    if (compare) {
      const prev = periodTotals(scope, spec, pFrom, pTo);
      stat.prev_events = prev.events;
      stat.prev_people = prev.people;
    }
    events.push(stat);
  }
  events.sort((a, b) => b.events - a.events || a.name.localeCompare(b.name));
  return { events, truncated: false };
}

function perPerson(t: Totals): number {
  return t.people === 0 ? 0 : Math.round((t.events / t.people) * 100) / 100;
}

function series(
  scope: Scope,
  spec: EventSpec | undefined,
  from: string,
  to: string,
  granularity: Granularity,
): EventPoint[] {
  const tz = scope.demo.app.timezone;
  if (granularity === "hour") {
    const out: EventPoint[] = [];
    for (const day of eachDay(from, to)) {
      const start = zonedMidnight(day, tz);
      // В сутках перехода на летнее время 23 или 25 часов, как у отчётов аудитории.
      const hours = Math.round((zonedMidnight(addDays(day, 1), tz) - start) / HOUR_MS);
      const starts = Array.from({ length: hours }, (_, h) => start + h * HOUR_MS);
      const weights = starts.map((t) => hourSessions(scope, day, localTime(t, tz).hour));
      const split = splitTotal(spec ? dayEvents(scope, spec, day) : 0, weights);
      for (const [h, t] of starts.entries()) {
        const local = localTime(t, tz);
        const events = split[h] ?? 0;
        const active = spec ? hourStats(scope, local.date, local.hour, t).active : 0;
        out.push({
          bucket: formatZoned(t, tz),
          events,
          people: spec ? Math.min(events, Math.round(active * spec.reach)) : 0,
        });
      }
    }
    return out;
  }
  return buckets(from, to, granularity).map((b) => {
    const totals = spec ? periodTotals(scope, spec, b.from, b.to) : { events: 0, people: 0 };
    return { bucket: formatZoned(zonedMidnight(b.start, tz), tz), ...totals };
  });
}

// propertyTotals — события и люди строк свойства: values по убыванию событий, missing — без ключа.
function propertyRows(
  spec: EventSpec,
  key: string,
  totals: Totals,
): { values: EventValueCount[]; missing: EventCount } {
  if (key !== spec.property.key) {
    return { values: [], missing: { ...totals } };
  }
  const missingEvents = Math.round(totals.events * spec.property.missing);
  const withKey = totals.events - missingEvents;
  const weights = spec.property.values.map(([, w]) => w);
  const counts = splitTotal(withKey, weights);
  const values: EventValueCount[] = [];
  spec.property.values.forEach(([value], i) => {
    const events = counts[i] ?? 0;
    if (events === 0) return;
    const share = withKey > 0 ? events / withKey : 0;
    values.push({
      value,
      events,
      people: Math.min(events, Math.max(1, Math.round(totals.people * share))),
    });
  });
  values.sort((a, b) => b.events - a.events || (a.value < b.value ? -1 : 1));
  const missingShare = totals.events > 0 ? missingEvents / totals.events : 0;
  return {
    values,
    missing: {
      events: missingEvents,
      people:
        missingEvents === 0
          ? 0
          : Math.min(missingEvents, Math.max(1, Math.round(totals.people * missingShare))),
    },
  };
}

// eventReport — страница события; имя без событий в каталоге или за период даёт нули.
export function eventReport(
  scope: Scope,
  name: string,
  from: string,
  to: string,
  granularity: Granularity,
  compare: boolean,
): EventReport {
  const spec = eventSpecs(scope).find((s) => s.name === name);
  const cur = spec ? periodTotals(scope, spec, from, to) : { events: 0, people: 0 };
  const summary: EventReport["summary"] = {
    events: cur.events,
    people: cur.people,
    events_per_person: perPerson(cur),
  };
  if (compare) {
    const [pFrom, pTo] = previousPeriod(from, to);
    const prev = spec ? periodTotals(scope, spec, pFrom, pTo) : { events: 0, people: 0 };
    summary.prev_events = prev.events;
    summary.prev_people = prev.people;
    summary.prev_events_per_person = perPerson(prev);
  }
  const keyEvents = spec
    ? cur.events - propertyRows(spec, spec.property.key, cur).missing.events
    : 0;
  return {
    name,
    summary,
    timeseries: series(scope, spec, from, to, granularity),
    property_keys:
      spec && keyEvents > 0
        ? [{ key: spec.property.key, events: keyEvents }].slice(0, EVENT_PROPERTY_KEYS_MAX)
        : [],
  };
}

// eventBreakdown — топ limit значений свойства, остальные одной строкой и события без ключа.
export function eventBreakdown(
  scope: Scope,
  name: string,
  property: string,
  from: string,
  to: string,
  limit: number,
): EventBreakdown {
  const spec = eventSpecs(scope).find((s) => s.name === name);
  if (!spec) return { property, values: [], missing: { events: 0, people: 0 } };
  const totals = periodTotals(scope, spec, from, to);
  const { values, missing } = propertyRows(spec, property, totals);
  const top = values.slice(0, limit);
  const rest = values.slice(limit);
  const out: EventBreakdown = { property, values: top, missing };
  if (rest.length > 0) {
    const events = rest.reduce((s, v) => s + v.events, 0);
    const people = rest.reduce((s, v) => s + v.people, 0);
    out.other = { events, people: Math.min(events, totals.people, people) };
  }
  return out;
}
