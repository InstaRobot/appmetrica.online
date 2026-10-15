"use client";

import {
  CircleCheckIcon,
  InfoIcon,
  Loader2Icon,
  OctagonXIcon,
  TriangleAlertIcon,
} from "lucide-react";
import type * as React from "react";
import { Toaster as Sonner, type ToasterProps } from "sonner";
import { useTheme } from "@/hooks/use-theme";

// Тема тостов следует атрибуту <html data-theme> (lib/theme.ts); next-themes не используется.
// Тосты — справа внизу, 3 с (§8.4).
const Toaster = ({ ...props }: ToasterProps) => {
  const theme = useTheme();
  return (
    <Sonner
      theme={theme}
      position="bottom-right"
      duration={3000}
      className="toaster group"
      icons={{
        success: <CircleCheckIcon className="size-4 text-positive" />,
        info: <InfoIcon className="size-4" />,
        warning: <TriangleAlertIcon className="size-4" />,
        error: <OctagonXIcon className="size-4" />,
        loading: <Loader2Icon className="size-4 animate-spin" />,
      }}
      style={
        {
          "--normal-bg": "var(--popover)",
          "--normal-text": "var(--popover-foreground)",
          "--normal-border": "var(--border)",
          "--border-radius": "var(--radius)",
        } as React.CSSProperties
      }
      {...props}
      toastOptions={{
        ...props.toastOptions,
        classNames: {
          toast: "cn-toast",
          ...props.toastOptions?.classNames,
        },
      }}
    />
  );
};

export { Toaster };
