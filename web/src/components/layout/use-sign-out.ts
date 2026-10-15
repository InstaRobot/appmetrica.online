"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { logout } from "@/lib/api/gen/endpoints/admin-auth/admin-auth";
import { toast } from "@/lib/toast";
import { useUiError } from "@/lib/use-ui-error";

// Выход из дашборда: меню «⋯» на десктопе и боковая панель на телефоне.
export function useSignOut() {
  const uiError = useUiError();
  const router = useRouter();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => logout(),
    onSuccess: () => {
      queryClient.clear();
      router.replace("/login");
    },
    onError: (error) => toast.error(uiError(error).message),
  });
}
