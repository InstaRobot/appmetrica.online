// Лендинг S-00 (components/landing) и его метаданные. Значения фактов и превью — иллюстрация, не данные.
export const landing = {
  meta: {
    title: "appmetrica.online — app analytics without an SDK",
    description: "App analytics without an SDK",
  },
  hero: {
    badge: "Data in the EU · no SDK · no sampling",
    title: "Analytics for your apps — with a single HTTP request",
    // {event} — имя события в <code>.
    lead: "A portfolio of all your apps, audience, retention and realtime — counted by people, not devices. One {event} event, and the app is already in your reports.",
    signIn: "Sign in to the dashboard",
    demo: "View demo data",
  },
  facts: {
    label: "Key limits",
    batch: { value: "≤ 500", label: "events per batch" },
    offline: { value: "7 days", label: "offline resend window" },
    storage: { value: "25 mo", label: "raw event retention" },
    latency: { value: "≤ 500 ms", label: "P99 ingest latency" },
  },
  features: {
    title: "What's inside the dashboard",
    lead: "A fixed set of reports you need every morning. No report builder, no extra settings.",
    ingest: {
      title: "HTTP event ingestion",
      text: "POST /v1/events: batches of up to 500 events, idempotency by insert_id, a 200 response only after the write to ClickHouse.",
    },
    portfolio: {
      title: "Portfolio",
      text: "All apps on one screen: DAU, new users, sessions, D1/D7 and a 30-day trend. A morning digest in Telegram.",
    },
    audience: {
      title: "Audience",
      text: "DAU/WAU/MAU, new users, sessions and session length. Breakdowns by version, OS, device, country and language, plus period comparison.",
    },
    retention: {
      title: "Retention",
      text: "Classic D0–D30 and W0–W12 cohorts defined the same way as in AppMetrica. A heatmap and an average curve.",
    },
    realtime: {
      title: "Realtime",
      text: "Users active right now, session starts per minute and a feed of the latest events that links to profiles.",
    },
    privacy: {
      title: "GDPR and privacy",
      text: "No IP addresses stored — only country and city. Per-person data deletion on request, a deletion log and a PII guard for user_id.",
    },
  },
  integration: {
    title: "Integrate in five minutes",
    lead: "Create an app, copy the write key and send your first batch. The format is the same for iOS, Android, web, bots and backends — no SDK needed.",
    stepsLabel: "Integration steps",
    steps: {
      create: "Create an app: name, platform, time zone. A pk_… write key is issued automatically.",
      send: "Send your first batch with session_start — curl, Swift, Kotlin, JavaScript or Go.",
      done: "The status changes from “Waiting for the first event” to “Received”, and the app shows up in the portfolio.",
    },
    snippetLabel: "Sample event ingestion request",
  },
  admin: {
    title: "Admin dashboard",
    text: "Passkey-only sign-in. Your users' private data never leaves the EU.",
    signIn: "Sign in",
  },
  preview: {
    label: "Sample report: the Notes iOS app over the last 30 days",
    period: "Last 30 days",
    active: "Active",
    new: "New",
    sessions: "Sessions",
    from: "30 days ago",
    chart: "Daily active users",
    to: "yesterday",
  },
};
