// Realtime S-07: счётчик активных, старты сессий по минутам и лента последних событий.
export const realtime = {
  title: "Realtime",
  pause: "Pause",
  resume: "Resume",
  stale: "Can't refresh the data. Showing data as of {time}.",
  counter: {
    title: "Active now",
    window: "in the last 30 min",
    updated: "updated {time}",
    paused: "paused",
    offline: "no connection",
    // Live-region для скринридеров, не чаще раза в 30 секунд.
    announce: "Active now: {count}",
  },
  chart: {
    title: "Session starts per minute",
    series: "Session starts",
    noData: "No data for the last 30 minutes",
    description: "Session starts per minute, {from}–{to}: {total} total, {last} in the last minute",
  },
  feed: {
    title: "Recent events",
    columns: {
      time: "Time",
      event: "Event",
      platform: "Platform",
      version: "Version",
      place: "Country / city",
      install: "Install",
      person: "Person",
    },
    install: "Install {id}",
    profile: "Profile {id}",
    empty: {
      title: "No events in the last 30 minutes",
      description:
        "Send a test event from a snippet on the Integration tab — it will show up here within 10 seconds.",
    },
  },
};
