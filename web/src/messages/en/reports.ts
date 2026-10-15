// Отчёты: общие элементы управления (окружение, период, детализация, фильтры), графики и экран S-05.
export const reports = {
  environment: "Environment",
  // Сервисный режим ?unit=installs (14-ux-ui-spec.md §16.7).
  unitBadge: {
    label: "Installs",
    hint: "Service mode: installs are counted instead of people. Remove ?unit=installs from the URL to go back.",
  },
  noData: "No data for the selected period",
  period: {
    label: "Period",
    presets: {
      today: "Today",
      yesterday: "Yesterday",
      "7d": "7 days",
      "30d": "30 days",
      "90d": "90 days",
    },
    pick: "Pick dates",
    picked: "Pick dates: {range}",
    apply: "Apply",
  },
  granularity: {
    label: "Granularity",
    options: { hour: "Hour", day: "Day", week: "Week", month: "Month" },
    changed: "Granularity changed to “{granularity}”",
  },
  compare: {
    label: "Compare",
    switch: "Compare with the previous period",
    unavailableInSplit: "Comparison isn't available in the by-platform mode",
  },
  platform: {
    label: "Platform",
    all: "All",
  },
  // Измерения разбивки и фильтров; порядок вкладок разбивки задаёт breakdown-table.
  dimensions: {
    app_version: "App version",
    os_name: "OS",
    os_version: "OS version",
    device_model: "Device",
    device_type: "Device type",
    country: "Country",
    city: "City",
    locale: "Language",
    platform: "Platform",
  },
  filters: {
    add: "Filter",
    search: "Find a value",
    searchPlaceholder: "Find a value…",
    loading: "Loading values…",
    empty: "Nothing found",
    remove: "Remove the “{dimension}” filter",
    reset: "Reset filters",
  },
  kpi: {
    section: "Key metrics",
    about: "What is “{metric}”?",
    median: "median {duration}",
    // Дельта к предыдущему периоду словами для скринридеров.
    delta: {
      up: "up {delta} vs. the previous period",
      down: "down {delta} vs. the previous period",
      flat: "no change vs. the previous period",
    },
    trend: {
      up: "Trend for the period: up",
      down: "Trend for the period: down",
      flat: "Trend for the period: flat",
    },
  },
  chart: {
    loading: "Loading chart…",
    showTable: "Show as table",
    hideTable: "Hide table",
    legend: "Chart legend",
    noEvents: "No events in the selected period",
    period: "Period",
    previousPeriod: "Previous period",
    previousValue: "Previous period: {value}",
    total: "Total",
    // Описание графика для скринридеров: «{title} {by}, период, максимум».
    titles: {
      active: "Active users chart",
      new: "New users chart",
      sessions: "Sessions chart",
      avg_duration_ms: "Average session duration chart",
    },
    by: { hour: "by hour", day: "by day", week: "by week", month: "by month" },
    heading: "{title} {by}",
    headingSplit: "{title} {by} per platform",
    peak: "peak {value} ({bucket})",
    noEventsShort: "no events",
  },
  rolling: {
    description: "DAU, WAU and MAU chart, {range}, last day: DAU {dau}, WAU {wau}, MAU {mau}",
    day: "Day",
  },
  breakdown: {
    title: "Breakdown",
    table: "Breakdown: {dimension}",
    dimension: "Dimension",
    metric: "Metric",
    value: "Value",
    share: "Share",
    notSet: "Not set",
    filterBy: "Filter: {dimension} {value}",
    sharesNote:
      "Shares can add up to more than 100%: a person may have used several values during the period.",
  },
  audience: {
    title: "Audience",
    copyRequest: "Copy as Read API request",
    requestCopied: "Request copied",
    lateData:
      "Data for the last 8 days may still be updated with events from offline queues (7-day window).",
    chartMetric: "Chart metric",
    chartMode: "Chart mode",
    byPlatform: "By platform",
    noEvents: {
      title: "No events yet",
      description: "Send your first event — snippets are on the Integration tab.",
      action: "Open integration",
    },
  },
};
