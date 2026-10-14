import type { ExternalToast, toast as sonnerToast } from "sonner";

// Тосты без sonner в initial JS (бюджет NFR-006, T-044): sonner грузится одним чанком вместе с
// <LazyToaster /> из корневого layout. Toaster sonner не показывает тосты, вызванные до его подписки,
// поэтому показ ждёт монтирования Toaster (markToasterMounted из components/common/mounted-toaster.tsx).
type Kind = "success" | "error" | "info";

let markMounted: () => void = () => {};
const mounted = new Promise<void>((resolve) => {
  markMounted = resolve;
});
let sonner: Promise<typeof sonnerToast> | undefined;

export function markToasterMounted(): void {
  markMounted();
}

function show(kind: Kind, message: string, options?: ExternalToast): void {
  sonner ??= Promise.all([import("sonner"), mounted]).then(([m]) => m.toast);
  void sonner.then((t) => t[kind](message, options));
}

export const toast = {
  success: (message: string, options?: ExternalToast) => show("success", message, options),
  error: (message: string, options?: ExternalToast) => show("error", message, options),
  info: (message: string, options?: ExternalToast) => show("info", message, options),
};
