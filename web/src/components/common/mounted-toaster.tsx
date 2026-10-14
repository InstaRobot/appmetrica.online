"use client";

import { useEffect } from "react";
import { Toaster } from "@/components/ui/sonner";
import { markToasterMounted } from "@/lib/toast";

// Toaster, после подписки которого lib/toast начинает показывать тосты: эффект дочернего Sonner
// (подписка на тосты) выполняется раньше эффекта родителя.
export function MountedToaster() {
  useEffect(markToasterMounted, []);
  return <Toaster />;
}
