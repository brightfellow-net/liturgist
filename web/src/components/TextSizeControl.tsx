// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { ErrorAlert } from "@/components/ErrorAlert";
import { api, call } from "@/lib/api";
import { meQuery } from "@/lib/queries";

const sizes = [
  { value: "normal", label: "A" },
  { value: "large", label: "A+" },
  { value: "larger", label: "A++" },
] as const;

// TextSizeControl is the "A · A+ · A++" switch of the reading pages. It saves
// the same preference as the profile page; AppLayout applies it from the cache.
export function TextSizeControl({ me }: { me: Me }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const current = me.user.preferences.text_size ?? "normal";
  const save = useMutation({
    mutationFn: (text_size: string) =>
      call(api.PATCH("/me", { body: { preferences: { text_size, ui_language: me.user.preferences.ui_language ?? null } } })),
    onSuccess: (user) => queryClient.setQueryData(meQuery.queryKey, (old) => old && { ...old, user }),
  });
  return (
    <div role="group" aria-label={t("profile.text_size")} className="flex flex-wrap items-center gap-1">
      {sizes.map((s) => (
        <button
          key={s.value}
          type="button"
          aria-pressed={current === s.value}
          aria-label={t(`profile.text_sizes.${s.value}`)}
          disabled={save.isPending}
          onClick={() => save.mutate(s.value)}
          className={"inline-flex min-h-12 min-w-12 items-center justify-center rounded-md border border-border px-3 " + (current === s.value ? "bg-muted font-semibold" : "hover:bg-muted")}
        >
          {s.label}
        </button>
      ))}
      <ErrorAlert error={save.error} />
    </div>
  );
}
