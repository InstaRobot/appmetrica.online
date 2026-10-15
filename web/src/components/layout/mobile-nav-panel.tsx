"use client";

import { LogOut, X } from "lucide-react";
import Link from "next/link";
import { Dialog as DialogPrimitive } from "radix-ui";
import { Button } from "@/components/ui/button";
import { useMessages } from "@/lib/i18n/client";
import { cn } from "@/lib/utils";
import { LanguageSwitcher } from "./language-switcher";
import type { MobileNavProps } from "./mobile-nav";
import { ThemeToggle } from "./theme-toggle";
import { useSignOut } from "./use-sign-out";

type PanelProps = MobileNavProps & { open: boolean; onOpenChange: (open: boolean) => void };

const rowClass =
  "flex min-h-11 items-center gap-2 rounded-md px-3 text-label-md text-fg-muted transition-colors hover:bg-subtle hover:text-foreground";

// Боковая панель навигации на телефоне: Radix Dialog справа на всю высоту. Переход по разделу закрывает её.
export function MobileNavPanel({ variant, items, demo = false, open, onOpenChange }: PanelProps) {
  const m = useMessages();
  const t = m.layout.mobileNav;

  return (
    <DialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-overlay bg-black/30" />
        <DialogPrimitive.Content
          aria-describedby={undefined}
          className="fixed inset-y-0 right-0 z-modal flex w-72 max-w-[85vw] flex-col gap-4 border-l border-border bg-card p-4 shadow-lg outline-none"
        >
          <div className="flex items-center justify-between">
            <DialogPrimitive.Title className="text-heading-3">{t.title}</DialogPrimitive.Title>
            <DialogPrimitive.Close asChild>
              <Button variant="ghost" size="icon-lg" aria-label={t.close} className="size-11">
                <X aria-hidden="true" className="size-5" />
              </Button>
            </DialogPrimitive.Close>
          </div>
          <nav aria-label={m.layout.header.nav}>
            <ul className="flex flex-col gap-1">
              {items.map((item) => (
                <li key={item.href}>
                  <Link
                    href={item.href}
                    aria-current={item.active ? "page" : undefined}
                    onClick={() => onOpenChange(false)}
                    className={cn(
                      rowClass,
                      item.active &&
                        "bg-selected text-fg-link hover:bg-selected hover:text-fg-link",
                    )}
                  >
                    {item.label}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
          <div className="flex items-center gap-2 border-t border-border pt-4">
            <LanguageSwitcher className="size-11" />
            <ThemeToggle />
          </div>
          {variant === "panel" ? (
            demo ? (
              <a href="/demo/exit" className={rowClass}>
                <LogOut aria-hidden="true" className="size-5" />
                {m.demo.exit}
              </a>
            ) : (
              <SignOutButton label={m.layout.userMenu.signOut} />
            )
          ) : null}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  );
}

function SignOutButton({ label }: { label: string }) {
  const signOut = useSignOut();
  return (
    <button
      type="button"
      disabled={signOut.isPending}
      onClick={() => signOut.mutate()}
      className={cn(rowClass, "w-full text-left disabled:opacity-50")}
    >
      <LogOut aria-hidden="true" className="size-5" />
      {label}
    </button>
  );
}
