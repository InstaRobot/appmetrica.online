// Нагрузочный смоук ingest (T-015, NFR-001: P99 ≤ 500 мс при 500 rps), k6 v2.3.0.
// Запуск: `make perf-ingest` (k6 в Docker против `make run-ingest-perf`), см. 22-test-strategy.md §11.1.
//
// Переменные окружения (не K6_*, кроме ключа: префикс K6_ k6 читает как свои опции):
//   K6_APP_KEY    — write-ключ pk_ (DEV_WRITE_KEY из `make seed`), обязателен;
//   INGEST_URL    — базовый URL ingest, по умолчанию http://localhost:8081;
//   PERF_RATE     — итераций (батчей) в секунду, по умолчанию 500;
//   PERF_DURATION — длительность, по умолчанию 10m.
import { check } from "k6";
import http from "k6/http";
import { Counter } from "k6/metrics";

const BATCH_SIZE = 10; // 5 пар session_start/session_end одной установки
const INSTALL_POOL = 100000;

const appKey = __ENV.K6_APP_KEY;
const ingestURL = `${__ENV.INGEST_URL || "http://localhost:8081"}/v1/events`;
const rate = Number(__ENV.PERF_RATE || 500);

// 5xx без Retry-After — клиент не знает, когда повторять: таких ответов быть не должно (NFR-001).
const serverErrorsWithoutRetryAfter = new Counter("server_errors_without_retry_after");

export const options = {
  scenarios: {
    ingest: {
      executor: "constant-arrival-rate",
      rate,
      timeUnit: "1s",
      duration: __ENV.PERF_DURATION || "10m",
      preAllocatedVUs: 500,
      maxVUs: 2000,
    },
  },
  thresholds: {
    "http_req_duration{expected_response:true}": ["p(99)<500"],
    http_req_failed: ["rate==0"],
    checks: ["rate>0.999"],
    server_errors_without_retry_after: ["count==0"],
    // Не хватило VU — нагрузка ниже заявленной, и замер не про 500 rps.
    dropped_iterations: [`rate<${rate * 0.01}`],
  },
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
};

export function setup() {
  if (!appKey) {
    throw new Error("K6_APP_KEY не задан: нужен write-ключ pk_ (DEV_WRITE_KEY после `make seed`)");
  }
}

function hex(n, width) {
  return n.toString(16).padStart(width, "0");
}

function randomUUID() {
  const r = () => hex(Math.floor(Math.random() * 0x10000), 4);
  return `${r()}${r()}-${r()}-4${r().slice(1)}-${hex(0x8000 | (Math.random() * 0x3fff), 4)}-${r()}${r()}${r()}`;
}

// installID — UUIDv4 из пула INSTALL_POOL: повторные батчи одной установки, как у живых устройств.
function installID(n) {
  return `00000000-0000-4000-8000-${hex(n, 12)}`;
}

function batch(now) {
  const install = installID(Math.floor(Math.random() * INSTALL_POOL));
  const events = [];
  for (let i = 0; i < BATCH_SIZE / 2; i++) {
    const session = randomUUID();
    const durationMs = 30000 + Math.floor(Math.random() * 600000);
    const start = new Date(now - durationMs - (BATCH_SIZE / 2 - i) * 1000);
    const end = new Date(start.getTime() + durationMs);
    events.push(
      {
        insert_id: randomUUID(),
        name: "session_start",
        timestamp: start.toISOString(),
        install_id: install,
        session_id: session,
      },
      {
        insert_id: randomUUID(),
        name: "session_end",
        timestamp: end.toISOString(),
        install_id: install,
        session_id: session,
        duration_ms: durationMs,
      },
    );
  }
  return {
    sent_at: new Date(now).toISOString(),
    context: {
      platform: "ios",
      os_name: "iOS",
      os_version: "26.1",
      app_version: "1.4.0",
      device_model: "iPhone17,1",
      device_type: "phone",
      locale: "ru-RU",
      environment: "production",
    },
    events,
  };
}

export default function () {
  const res = http.post(ingestURL, JSON.stringify(batch(Date.now())), {
    headers: { "Content-Type": "application/json", "X-App-Key": appKey },
  });
  if (res.status >= 500 && !res.headers["Retry-After"]) {
    serverErrorsWithoutRetryAfter.add(1);
  }
  check(res, {
    "status 200": (r) => r.status === 200,
    [`accepted == ${BATCH_SIZE}`]: (r) => r.status === 200 && r.json("accepted") === BATCH_SIZE,
  });
}
