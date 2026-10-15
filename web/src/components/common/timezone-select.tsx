"use client";

import { Check, ChevronsUpDown } from "lucide-react";
import { Suspense, useMemo, useState } from "react";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSkeleton,
} from "@/components/common/lazy-command";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useMessages } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";

type TimezoneSelectProps = {
  value: string;
  onChange: (tz: string) => void;
  id?: string;
  invalid?: boolean | undefined;
  describedBy?: string | undefined;
  required?: boolean;
  /** Вызывается при закрытии списка — для валидации поля формы «on blur». */
  onClose?: () => void;
  ref?: React.Ref<HTMLButtonElement>;
};

let zones: string[] | undefined;

// IANA-зоны браузера; UTC добавляется явно — V8 не включает его в supportedValuesOf.
function timezones(): string[] {
  zones ??= ["UTC", ...Intl.supportedValuesOf("timeZone").filter((tz) => tz !== "UTC")];
  return zones;
}

// isTimezone принимает имена IANA, которые знает Intl, включая алиасы (Europe/Kyiv при Europe/Kiev в списке).
export function isTimezone(tz: string): boolean {
  if (!tz) return false;
  try {
    new Intl.DateTimeFormat("en-US", { timeZone: tz });
    return true;
  } catch {
    return false;
  }
}

const labels = new Map<string, string>();

// «Europe/Moscow (UTC+3)», «Asia/Kolkata (UTC+5:30)», «UTC (UTC)»: текущее смещение зоны.
export function timezoneLabel(tz: string): string {
  if (!isTimezone(tz)) return tz;
  let label = labels.get(tz);
  if (label === undefined) {
    const offset = new Intl.DateTimeFormat("en-US", { timeZone: tz, timeZoneName: "shortOffset" })
      .formatToParts(new Date())
      .find((p) => p.type === "timeZoneName")?.value;
    const utc =
      !offset || offset === "GMT" || offset === "GMT+0" ? "UTC" : offset.replace("GMT", "UTC");
    label = `${tz} (${utc.replace("-", "−")})`;
    labels.set(tz, label);
  }
  return label;
}

// Выбор часового пояса (14-ux-ui-spec.md §16.13): Popover + Command с поиском по имени зоны.
export function TimezoneSelect({
  value,
  onChange,
  id,
  invalid,
  describedBy,
  required,
  onClose,
  ref,
}: TimezoneSelectProps) {
  const m = useMessages().common.timezone;
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");

  const needle = search.trim().toLowerCase().replaceAll(" ", "_");
  const visible = useMemo(
    () => (needle ? timezones().filter((tz) => tz.toLowerCase().includes(needle)) : timezones()),
    [needle],
  );

  const toggle = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setSearch("");
      onClose?.();
    }
  };

  return (
    <Popover open={open} onOpenChange={toggle}>
      <PopoverTrigger asChild>
        <Button
          ref={ref}
          id={id}
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          aria-required={required || undefined}
          className={cn("w-full justify-between font-normal", !value && "text-muted-foreground")}
        >
          <span className="truncate">{value ? timezoneLabel(value) : m.placeholder}</span>
          <ChevronsUpDown aria-hidden="true" className="text-fg-subtle" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-(--radix-popover-trigger-width) p-0">
        <Suspense fallback={<CommandSkeleton />}>
          <Command shouldFilter={false}>
            <CommandInput placeholder={m.search} value={search} onValueChange={setSearch} />
            <CommandList>
              <CommandEmpty>{m.notFound}</CommandEmpty>
              {visible.length > 0 ? (
                <CommandGroup>
                  {visible.map((tz) => (
                    <CommandItem
                      key={tz}
                      value={tz}
                      onSelect={() => {
                        onChange(tz);
                        toggle(false);
                      }}
                    >
                      <span className="truncate">{timezoneLabel(tz)}</span>
                      <Check
                        aria-hidden="true"
                        className={cn("ml-auto", tz === value ? "opacity-100" : "opacity-0")}
                      />
                    </CommandItem>
                  ))}
                </CommandGroup>
              ) : null}
            </CommandList>
          </Command>
        </Suspense>
      </PopoverContent>
    </Popover>
  );
}
