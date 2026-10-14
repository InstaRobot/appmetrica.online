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
import { toUiError } from "@/lib/errors";
import { toast } from "@/lib/toast";

// Меню «⋯» шапки (14-ux-ui-spec.md §6.1): выход из дашборда.
export function UserMenu() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const signOut = useMutation({
    mutationFn: () => logout(),
    onSuccess: () => {
      queryClient.clear();
      router.replace("/login");
    },
    onError: (error) => toast.error(toUiError(error).message),
  });

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon-lg" aria-label="Меню пользователя">
          <Ellipsis aria-hidden="true" className="size-5" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem disabled={signOut.isPending} onSelect={() => signOut.mutate()}>
          <LogOut aria-hidden="true" />
          Выйти
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
