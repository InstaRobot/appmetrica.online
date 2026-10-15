// Профили людей: поиск S-08, карточка S-09 и удаление данных по GDPR (FR-016, FR-019).
export const profiles = {
  title: "Profiles",
  profileTitle: "Profile",
  search: {
    label: "install_id or user_id",
    submit: "Find",
    hint: "Exact match search. A person_key like u:… or i:… works too.",
    required: "Enter an install_id or user_id",
    tooLong: "200 characters at most",
  },
  results: {
    caption: "Results",
    columns: {
      person: "Person",
      userId: "user_id",
      installs: "Devices",
      firstSeen: "First visit",
      lastSeen: "Last",
    },
    profileLink: "Profile {key}",
    emptyTitle: "Nothing found",
    emptyDescription:
      "Check the value: search only works with an exact install_id or user_id. Data older than 25 months is deleted.",
  },
  notFound: {
    title: "Profile not found",
    description: "The data may have been deleted or hasn't arrived yet.",
  },
  backToSearch: "Back to search",
  installsTitle: "Devices",
  sessionsTitle: "Sessions",
  deleted: "Data deleted",
  card: {
    copyKey: "Copy person_key",
    deleteData: "Delete data",
    firstSeen: "First visit:",
    lastSeen: "Last:",
    deletionStatus: "Deletion status",
    deletionStatusLabel: "Deletion status:",
  },
  installs: {
    caption: "Devices",
    columns: {
      installId: "Install ID",
      platform: "Platform",
      device: "Device",
      os: "OS",
      version: "Version",
      geo: "Location",
      firstSeen: "First",
      lastSeen: "Last",
    },
  },
  sessions: {
    empty: "No sessions",
    caption: "Sessions",
    columns: {
      start: "Start",
      duration: "Duration",
      version: "Version",
      platform: "Platform",
      install: "Install",
    },
    noEnd: "— (no session_end)",
    more: "Show more",
  },
  deleteDialog: {
    title: "Delete all data for this person?",
    description:
      "All events and device links for {key} in the app “{app}” will be deleted. Aggregates for closed days don't change. Deletion runs in the background for up to 24 hours and can't be undone.",
    confirmText: "DELETE",
    confirm: "Delete data",
    created: "Deletion request created",
  },
  deletionStatus: {
    pending: "Queued",
    running: "In progress",
    done: "Deleted",
    failed: "Failed",
  },
};
