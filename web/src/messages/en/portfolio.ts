// Портфель S-03: таблица всех приложений за дату, переключатель даты и архив.
export const portfolio = {
  title: "Portfolio",
  addApp: "Add app",
  date: {
    previous: "Previous day",
    next: "Next day",
    pick: "Pick a date: {date}",
  },
  noEvents: "No events on {date}. Check the Integration tab of your apps.",
  noEventsYet: "No events on {date}, but they are already coming in today.",
  showToday: "Show today",
  empty: {
    title: "No apps yet",
    description: "Add your first app to get a key and ready-made snippets for sending events.",
  },
  showArchived: "Show archived ({count})",
  archive: {
    title: "Archive",
    name: "Name",
    platforms: "Platforms",
    archivedAt: "Archived on",
    actions: "Actions",
    open: "Open",
  },
  table: {
    caption: "App portfolio for {date}",
    columns: {
      name: "App",
      platforms: "Platforms",
      today: "Today",
      todayHint: "Partial day: data since 00:00 in the app's time zone",
      dau: "DAU {date}",
      delta: "Δ vs {date}",
      new: "New {date}",
      sessions: "Sessions {date}",
      sparkline: "30 days",
    },
    // Тренд 30 дней для скринридеров: средняя первой недели против последней.
    sort: {
      label: "Sort by",
      descending: "Descending",
    },
    trend: {
      up: "30-day trend: up",
      down: "30-day trend: down",
      flat: "30-day trend: flat",
    },
  },
  shares: {
    overlap: "A person on several platforms is counted in each",
  },
};
