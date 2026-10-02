// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
// Based on shadcn/ui (MIT). Errors are announced (role="alert"); other
// messages are polite (role="status").
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

export function Alert({ className, variant = "info", ...props }: ComponentProps<"div"> & { variant?: "info" | "error" }) {
  return (
    <div
      role={variant === "error" ? "alert" : "status"}
      className={cn(
        "rounded-md border px-4 py-3",
        variant === "error" ? "border-destructive text-destructive" : "border-border bg-muted",
        className,
      )}
      {...props}
    />
  );
}
