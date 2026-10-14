type TimezoneSelectProps = {
  value: string;
  onChange: (tz: string) => void;
  id?: string;
  invalid?: boolean;
  describedBy?: string;
};

export function isTimezone(_tz: string): boolean {
  return false;
}

export function timezoneLabel(tz: string): string {
  return tz;
}

// TODO(T-037)
export function TimezoneSelect(_props: TimezoneSelectProps) {
  return null;
}
