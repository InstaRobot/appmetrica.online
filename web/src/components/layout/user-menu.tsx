"use client";

import { Ellipsis, LogOut } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { useMessages } from "@/lib/i18n/client";
import { useSignOut } from "./use-sign-out";

// Меню «⋯» шапки (14-ux-ui-spec.md §6.1): выход из дашборда; в демо-режиме — выход из демо
// (полная навигация на /demo/exit: route handler снимает cookie и уводит на лендинг).
export function UserMenu({ demo = false }: { demo?: boolean }) {
  const m = useMessages();
  const signOut = useSignOut();

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-lg" aria-label={m.layout.userMenu.label}>
          <Ellipsis aria-hidden="true" className="size-5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {demo ? (
          <DropdownMenuItem asChild>
            <a href="/demo/exit">
              <LogOut aria-hidden="true" />
              {m.demo.exit}
            </a>
          </DropdownMenuItem>
        ) : (
          <DropdownMenuItem disabled={signOut.isPending} onSelect={() => signOut.mutate()}>
            <LogOut aria-hidden="true" />
            {m.layout.userMenu.signOut}
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
