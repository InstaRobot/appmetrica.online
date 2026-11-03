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
};
