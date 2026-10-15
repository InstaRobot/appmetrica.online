"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Ellipsis, LogOut } from "lucide-react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { logout } from "@/lib/api/gen/endpoints/admin-auth/admin-auth";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";

// Меню «⋯» шапки (14-ux-ui-spec.md §6.1): выход из дашборда; в демо-режиме — выход из демо
// (полная навигация на /demo/exit: route handler снимает cookie и уводит на лендинг).
export function UserMenu({ demo = false }: { demo?: boolean }) {
  const uiError = useUiError();
  const router = useRouter();
  const queryClient = useQueryClient();
  const signOut = useMutation({
    mutationFn: () => logout(),
    onSuccess: () => {
      queryClient.clear();
      router.replace("/login");
    },
    onError: (error) => toast.error(uiError(error).message),
  });

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-lg" aria-label="Меню пользователя">
          <Ellipsis aria-hidden="true" className="size-5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {demo ? (
          <DropdownMenuItem asChild>
            <a href="/demo/exit">
              <LogOut aria-hidden="true" />
              Выйти из демо
            </a>
          </DropdownMenuItem>
        ) : (
          <DropdownMenuItem disabled={signOut.isPending} onSelect={() => signOut.mutate()}>
            <LogOut aria-hidden="true" />
            Выйти
          </DropdownMenuItem>
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
