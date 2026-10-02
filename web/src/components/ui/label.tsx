// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
// Based on shadcn/ui (MIT).
import type { ComponentProps } from "react";
import { cn } from "@/lib/utils";

export function Label({ className, ...props }: ComponentProps<"label">) {
  return <label className={cn("block text-base font-medium", className)} {...props} />;
}
