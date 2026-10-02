// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
// Based on shadcn/ui (MIT).
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

export const fieldClass =
  "block min-h-12 w-full rounded-md border border-input bg-background px-3 py-2 text-base " +
  "focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring " +
  "aria-invalid:border-destructive";

export function Input({ className, ...props }: ComponentProps<"input">) {
  return <input className={cn(fieldClass, className)} {...props} />;
}

export function Select({ className, ...props }: ComponentProps<"select">) {
  return <select className={cn(fieldClass, className)} {...props} />;
}
