// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { ChurchView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { api, call } from "@/lib/api";
import { fieldErrors } from "@/lib/errors";
import { churchQuery, meQuery } from "@/lib/queries";

const types = ["image/png", "image/jpeg", "image/webp"];
const maxBytes = 2 * 1024 * 1024; // the server's limit (15 §2, L-2)

// base64 returns the contents of a file as base64, without the "data:" prefix.
function base64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).replace(/^data:[^,]*,/, ""));
    reader.onerror = () => reject(reader.error ?? new Error("read failed"));
    reader.readAsDataURL(file);
  });
}

// ChurchLogoSection lets a church admin set or remove the logo (15 §2, L-6).
// The browser checks type and size for a quick answer; the server decides.
export function ChurchLogoSection({ church }: { church: ChurchView }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [problem, setProblem] = useState<string | null>(null);
  const done = async (updated: ChurchView) => {
    queryClient.setQueryData(churchQuery.queryKey, updated);
    await queryClient.invalidateQueries({ queryKey: meQuery.queryKey }); // the header shows the logo
  };
  const upload = useMutation({
    mutationFn: async (file: File) => call(api.PUT("/church/logo", { body: { image: await base64(file) } })),
    onSuccess: done,
    onError: (err) => setProblem(fieldErrors(err).includes("image") ? t("church.logo_invalid") : null),
  });
  const remove = useMutation({ mutationFn: () => call(api.DELETE("/church/logo")), onSuccess: done });
  const busy = upload.isPending || remove.isPending;

  const choose = (file: File | undefined) => {
    setProblem(null);
    remove.reset();
    if (input.current) input.current.value = ""; // the same file can be chosen again
    if (!file) return;
    if (!types.includes(file.type)) return setProblem(t("church.logo_type"));
    if (file.size > maxBytes) return setProblem(t("church.logo_too_big"));
    upload.mutate(file);
  };

  return (
    <section aria-labelledby="logo-heading" className="max-w-xl space-y-3">
      <h2 id="logo-heading" className="text-lg font-semibold">{t("church.logo")}</h2>
      {church.logo_url ? (
        <img src={church.logo_url} alt={t("church.logo_current")} className="max-h-32 max-w-full rounded border border-border bg-white object-contain p-2" />
      ) : (
        <p className="text-muted-foreground">{t("church.logo_none")}</p>
      )}
      <p className="text-sm text-muted-foreground">{t("church.logo_hint")}</p>
      {problem && <Alert variant="error">{problem}</Alert>}
      <ErrorAlert error={problem ? null : upload.error ?? remove.error} />
      <div className="flex flex-wrap items-center gap-3">
        <label className="inline-flex min-h-12 cursor-pointer items-center rounded-md border border-border px-4 font-medium hover:bg-muted focus-within:ring-2 focus-within:ring-ring">
          {upload.isPending ? t("church.logo_uploading") : t("church.logo_choose")}
          <input
            ref={input}
            type="file"
            accept={types.join(",")}
            disabled={busy}
            className="sr-only"
            onChange={(e) => choose(e.target.files?.[0])}
          />
        </label>
        {church.logo_url && (
          <Button variant="outline" disabled={busy} onClick={() => remove.mutate()}>{t("church.logo_remove")}</Button>
        )}
      </div>
    </section>
  );
}
