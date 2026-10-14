// «Живые» отчёты демо-режима: realtime (S-07) и здоровье интеграции (S-10). События лежат на сетке
// абсолютных минут, поэтому при опросе каждые 5 с лента сдвигается, а не перегенерируется. Отказы —
// фоновая доля rejectRate и всплески каталога (rejectBursts).
import type {
  Environment,
  IntegrationHealth,
  IntegrationHour,
  IntegrationReason,
  IntegrationVersion,
  Realtime,
  RecentEvent,
} from "@/lib/api/gen/model";
import type { RejectBurst } from "./catalog";
import {
  type DemoApp,
  dayStats,
  dimEntries,
  hourSessions,
  makeScope,
  type Scope,
  versionSessionShare,
  versionShares,
} from "./model";
import { appPersons, POOL_SIZE } from "./profiles";
import { jitter, rand, roundRandom, seeded, splitTotal, uuidFrom } from "./random";
import { formatZoned, HOUR_MS, isoUtc, localTime, MINUTE_MS } from "./time";

const REALTIME_MINUTES = 30;
const RECENT_LIMIT = 50;
const RECENT_LOOKBACK_MINUTES = 360;

// isLive — приложение принимает события сегодня: уже создано и не в архиве.
function isLive(demo: DemoApp, today: string): boolean {
  return today >= demo.firstDay && (demo.lastDay === null || today <= demo.lastDay);
}

// sessionsPerMinute — ожидаемое число начатых сессий в минуту момента t (по суточной модели и часу).
function sessionsPerMinute(scope: Scope, t: number): number {
  const local = localTime(t, scope.demo.app.timezone);
  return hourSessions(scope, local.date, local.hour) / 60;
}

// recentEvents — лента realtime; platforms (не пусто) оставляет события этих платформ, не сдвигая остальные.
function recentEvents(
  demo: DemoApp,
  env: Environment,
  now: number,
  platforms: readonly string[],
): RecentEvent[] {
  const id = demo.app.id;
  const persons = appPersons(demo);
  const curated = persons.filter((p) => p.curated);
  const testers = curated.filter((p) => p.devShare > 0);
  const devPeople = testers.length > 0 ? testers : curated;
  const out: RecentEvent[] = [];
  let minute = Math.floor(now / MINUTE_MS);
  for (let back = 0; back < RECENT_LOOKBACK_MINUTES && out.length < RECENT_LIMIT; back++) {
    const r = seeded(id, env, "rt", minute);
    const count = env === "production" ? 7 + Math.floor(r() * 7) : r() < 0.3 ? 1 : 0;
    for (let j = 0; j < count; j++) {
      const received = minute * MINUTE_MS + Math.floor(r() * MINUTE_MS);
      const lag = 120 + Math.floor(r() * 1500);
      const late = r() < 0.04 ? Math.floor(r() * 3 * HOUR_MS) : 0;
      const name = r() < 0.5 ? "session_start" : "session_end";
      const fromCurated = r() < 0.1;
      const personDraw = r();
      const installDraw = r();
      if (received > now) continue;
      const person =
        env === "development"
          ? devPeople[Math.floor(personDraw * devPeople.length)]
          : fromCurated
            ? curated[Math.floor(personDraw * curated.length)]
            : persons[curated.length + Math.floor(personDraw * POOL_SIZE)];
      const install = person?.installs[Math.floor(installDraw * person.installs.length)];
      if (!person || !install) continue;
      if (platforms.length > 0 && !platforms.includes(install.platform)) continue;
      out.push({
        insert_id: uuidFrom(id, env, "event", minute, j),
        event_time: isoUtc(received - lag - late),
        received_at: isoUtc(received),
        name,
        environment: env,
        platform: install.platform,
        app_version: install.app_version,
        os_name: install.os_name,
        os_version: install.os_version,
        device_model: install.device_model,
        country: install.country,
        city: install.city,
        install_id: install.installId,
        person_key: person.key,
      });
    }
    minute--;
  }
  return out
    .sort(
      (a, b) =>
        b.received_at.localeCompare(a.received_at) || a.insert_id.localeCompare(b.insert_id),
    )
    .slice(0, RECENT_LIMIT);
}

// realtime — срез «прямо сейчас»; platforms — фильтр platform (пусто — все платформы).
export function realtime(
  demo: DemoApp,
  env: Environment,
  now: number,
  platforms: readonly string[] = [],
): Realtime {
  const scope = makeScope(demo, now, { env, filters: { platform: platforms } });
  const tz = demo.app.timezone;
  const id = demo.app.id;
  const live = isLive(demo, scope.today);
  const lastFull = Math.floor(now / MINUTE_MS) * MINUTE_MS;
  const perMinute = Array.from({ length: REALTIME_MINUTES }, (_, i) => {
    const t = lastFull - (REALTIME_MINUTES - i) * MINUTE_MS;
    const expected = live ? sessionsPerMinute(scope, t) * jitter(0.5, id, env, t, "pm") : 0;
    return { minute: formatZoned(t, tz), sessions_started: roundRandom(expected, id, env, t) };
  });
  const avgMinutes = (dayStats(scope, scope.today, true).durationMs ?? 0) / MINUTE_MS;
  const concurrent = live
    ? sessionsPerMinute(scope, now) * avgMinutes * 1.15 * jitter(0.2, id, env, lastFull, "an")
    : 0;
  return {
    active_now: roundRandom(concurrent, id, env, lastFull, "active"),
    per_minute: perMinute,
    recent_events: live ? recentEvents(demo, env, now, platforms) : [],
    generated_at: isoUtc(now),
  };
}

// --- здоровье интеграции ---

const REJECTION_MESSAGES: Readonly<Record<string, string>> = {
  duration_required: "Для session_end обязательно поле duration_ms (0..86400000).",
  event_too_old: "Событие старше 7 дней после коррекции часов.",
  invalid_field: "timestamp должен быть в формате RFC 3339 с offset.",
  missing_field: "Поле session_id обязательно.",
  install_rate_limited: "Слишком много событий с этого устройства. Повторите позже.",
  user_id_looks_like_pii: "user_id похож на email или телефон. Передавайте непрозрачный ID.",
  unknown_event_name: "Допустимые имена: session_start, session_end.",
  app_archived: "Приложение в архиве, приём событий остановлен.",
};

const APP_VERSION_NOT_STRING = "context.app_version должен быть строкой.";

// examplePayload — payload_sample отказа в каноничном виде ingest: без IP, user_id-PII уже заменён.
// burst с версией — релиз, который шлёт context.app_version числом.
function examplePayload(
  demo: DemoApp,
  code: string,
  at: number,
  version: string,
  burst: RejectBurst | undefined,
): string {
  const id = demo.app.id;
  const seg = demo.model.segments[0];
  const os = seg ? (dimEntries(seg, "os_name")[0]?.value ?? "") : "";
  const event: Record<string, unknown> = {
    insert_id: uuidFrom(id, code, "insert"),
    name: "session_start",
    timestamp: isoUtc(at - 850),
    install_id: uuidFrom(id, code, "install"),
    session_id: uuidFrom(id, code, "session"),
  };
  switch (code) {
    case "duration_required":
      event.name = "session_end";
      break;
    case "event_too_old":
      event.timestamp = isoUtc(at - 9 * 24 * HOUR_MS - 3_812_000);
      break;
    case "invalid_field":
      if (!burst?.version) event.timestamp = "14.10.2026 12:03:11";
      break;
    case "missing_field":
      delete event.session_id;
      break;
    case "user_id_looks_like_pii":
      event.user_id = "[redacted]";
      break;
    case "unknown_event_name":
      event.name = "button_click";
      break;
  }
  event.context = {
    platform: demo.app.platform,
    os_name: os,
    app_version: burst?.version ? Number.parseFloat(burst.version) : version,
    environment: "production",
  };
  if (code === "app_archived") {
    return JSON.stringify({ sent_at: isoUtc(at - 120), events: [event] });
  }
  return JSON.stringify(event);
}

export function integrationHealth(demo: DemoApp, hours: number, now: number): IntegrationHealth {
  const id = demo.app.id;
  const tz = demo.app.timezone;
  const prod = makeScope(demo, now);
  const dev = makeScope(demo, now, { env: "development" });
  const local = localTime(now, tz);
  const hourStart = now - (now % 1000) - (local.minute * 60 + local.second) * 1000;
  const first = hourStart - (hours - 1) * HOUR_MS;
  const archived = demo.lastDay !== null;
  let baseTotal = 0;

  const bursts = demo.model.rejectBursts;
  const burstTotals: number[] = bursts.map(() => 0);
  let lastRejectedHour: number | null = null;
  const hourly: IntegrationHour[] = [];
  for (let i = 0; i < hours; i++) {
    const t = first + i * HOUR_MS;
    const at = localTime(t, tz);
    const elapsed = Math.min(1, (now - t) / HOUR_MS);
    const events =
      (hourSessions(prod, at.date, at.hour) + hourSessions(dev, at.date, at.hour)) * 2 * elapsed;
    const accepted = Math.round(events * jitter(0.1, id, t, "acc"));
    let rejected = archived
      ? roundRandom(1.4 * elapsed * jitter(1, id, t, "rej"), id, t, "rej")
      : roundRandom(accepted * demo.model.rejectRate * jitter(0.8, id, t, "rej"), id, t, "rej");
    const base = rejected;
    bursts.forEach((b, j) => {
      if (at.date < b.from || at.date >= b.until) return;
      const share = b.version ? versionSessionShare(prod, at.date, b.version) : 1;
      const n = roundRandom(accepted * b.rate * share * jitter(0.3, id, t, b.code), id, t, b.code);
      burstTotals[j] = (burstTotals[j] ?? 0) + n;
      rejected += n;
    });
    if (rejected > 0) lastRejectedHour = t;
    hourly.push({ hour: formatZoned(t, tz), accepted, rejected });
    baseTotal += base;
  }
  const acceptedTotal = hourly.reduce((s, h) => s + h.accepted, 0);
  const rejectedTotal = hourly.reduce((s, h) => s + h.rejected, 0);

  const latestReject =
    lastRejectedHour === null
      ? now
      : lastRejectedHour +
        Math.floor(
          rand(id, "last-reject", lastRejectedHour) * Math.min(HOUR_MS, now - lastRejectedHour),
        );
  const versions = versionShares(demo.model, local.date).slice(0, 4);
  const oldest = versions.at(-1)?.value ?? "";

  const codes = demo.model.rejections.map(([code]) => code);
  for (const b of bursts) if (!codes.includes(b.code)) codes.push(b.code);
  const baseCounts = splitTotal(
    baseTotal,
    demo.model.rejections.map(([, w]) => w),
  );
  const reasons: IntegrationReason[] = codes
    .map((code, i) => {
      const own = baseCounts[i] ?? 0;
      // Пример — от всплеска, если его отказов больше фоновых.
      const burst = bursts
        .map((b, j) => ({ b, n: burstTotals[j] ?? 0 }))
        .filter((x) => x.b.code === code && x.n > own)
        .sort((x, y) => y.n - x.n)[0]?.b;
      const fromBursts = bursts.reduce(
        (s, b, j) => s + (b.code === code ? (burstTotals[j] ?? 0) : 0),
        0,
      );
      const lastSeen = Math.max(
        first,
        latestReject - i * 7 * MINUTE_MS - Math.floor(rand(id, code, "seen") * 5 * MINUTE_MS),
      );
      return {
        code,
        count: own + fromBursts,
        last_seen: isoUtc(lastSeen),
        example_payload: examplePayload(demo, code, lastSeen, oldest, burst),
        example_message:
          burst?.version !== undefined ? APP_VERSION_NOT_STRING : (REJECTION_MESSAGES[code] ?? ""),
      };
    })
    .filter((r) => r.count > 0)
    .sort((a, b) => b.count - a.count || a.code.localeCompare(b.code));

  const acceptedByVersion = splitTotal(
    acceptedTotal,
    versions.map((v) => v.weight),
  );
  // Отказы всплесков с версией — у этой версии, остальные — больше у старых версий.
  const versionBursts = versions.map((v) =>
    bursts.reduce((s, b, j) => s + (b.version === v.value ? (burstTotals[j] ?? 0) : 0), 0),
  );
  const rejectedByVersion = splitTotal(
    rejectedTotal - versionBursts.reduce((s, n) => s + n, 0),
    versions.map((v, i) => v.weight * (i + 1) ** 2),
  );
  const freshest = archived
    ? latestReject
    : now - 1_500 - Math.floor(rand(id, "fresh", Math.floor(now / MINUTE_MS)) * 3_000);
  const versionRows: IntegrationVersion[] = versions.map((v, i) => ({
    app_version: v.value,
    accepted: acceptedByVersion[i] ?? 0,
    rejected: (rejectedByVersion[i] ?? 0) + (versionBursts[i] ?? 0),
    last_seen: isoUtc(
      Math.max(first, freshest - i * (40 + Math.floor(rand(id, v.value, "seen") * 30)) * MINUTE_MS),
    ),
  }));

  return {
    hours,
    first_event_at: isoUtc(Date.parse(demo.app.created_at) + 17 * MINUTE_MS + 3_512),
    accepted_total: acceptedTotal,
    rejected_total: rejectedTotal,
    hourly,
    reasons,
    versions: versionRows,
  };
}
