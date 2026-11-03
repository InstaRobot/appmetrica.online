// Вкладка «События»: список пользовательских событий приложения. Имена событий не переводятся.
export const events = {
  title: "Events",
  table: {
    label: "Custom events",
    name: "Event",
    events: "Events",
    people: "People",
    installs: "Installs",
    delta: "Change",
    deltaHint: "Change in the number of events against the previous period of the same length",
    perPerson: "Per person",
    perPersonHint: "Events divided by people",
  },
  truncated: "Only the first 500 events are shown. Narrow the period or the filters.",
  empty: {
    title: "No custom events yet",
    description:
      "Events your app sends with a name of its own show up here. System events such as session_start are not listed.",
    action: "How to send events",
  },
  event: {
    back: "Events",
    notFound: {
      title: "Event not found",
      description:
        "This name is not a custom event of the app. System events such as session_start are not shown here.",
      action: "Back to events",
    },
    totals: "Event totals",
    kpi: {
      events: "Events",
      people: "People",
      installs: "Installs",
      perPerson: "Events per person",
      perPersonHint: "Events divided by people",
    },
    chart: {
      metric: "Chart metric",
      events: "Events",
      people: "People",
      installs: "Installs",
    },
    breakdown: {
      title: "Properties",
      property: "Property",
      table: "{event} by {property}",
      value: "Value",
      events: "Events",
      people: "People",
      installs: "Installs",
      share: "Share",
      other: "Other",
      missing: "No property",
      noProperties: "This event has no properties in the period.",
      empty: "No events with this property in the period.",
      more: "Show more",
    },
  },
};
