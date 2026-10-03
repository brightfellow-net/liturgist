// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from "react";
import { Link, Navigate, useNavigate, useOutletContext } from "react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { ImportRejectedView, Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { importLimits, readFiles, reasonText, type PickedFile } from "@/lib/imports";
import { songLanguages } from "@/lib/library";
import { hasScope } from "@/lib/scopes";
import { importPath, paths } from "../paths";

const textarea = "block min-h-12 w-full rounded-md border border-input bg-background px-3 py-2 text-base aria-invalid:border-destructive";

type Format = "paste" | "openlyrics" | "chordpro";
type Sent = { format: Format; language?: (typeof songLanguages)[number]; files: { name: string; text: string }[] };

// ImportPage is step 1 of importing songs: three ways to hand over lyrics.
// Nothing is saved to the library here; the review page comes next (08 §6).
export function ImportPage() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  // skipped are the files the browser could not send; the review page lists
  // them with the files the server rejected.
  const create = useMutation({
    mutationFn: (v: { body: Sent; skipped: ImportRejectedView[] }) => call(api.POST("/imports", { body: v.body })),
    onSuccess: async (batch, v) => {
      await queryClient.invalidateQueries({ queryKey: ["imports"] });
      void navigate(importPath(batch.id), { state: { rejected: [...v.skipped, ...(batch.rejected ?? [])] } });
    },
  });

  if (!hasScope(me, "library.edit")) return <Navigate to={paths.library} replace />;

  return (
    <div className="max-w-3xl space-y-8">
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold">{t("import.title")}</h1>
        <p>{t("import.intro")}</p>
        <Link className="underline" to={paths.library}>{t("library.back")}</Link>
      </div>
      <ErrorAlert error={create.error} />
      <PasteForm busy={create.isPending} onSend={(body) => create.mutate({ body, skipped: [] })} />
      <FilesForm format="openlyrics" busy={create.isPending} onSend={(body, skipped) => create.mutate({ body, skipped })} />
      <FilesForm format="chordpro" busy={create.isPending} onSend={(body, skipped) => create.mutate({ body, skipped })} />
    </div>
  );
}

function PasteForm({ busy, onSend }: { busy: boolean; onSend: (b: Sent) => void }) {
  const { t, i18n } = useTranslation();
  const [errors, setErrors] = useState<{ title?: string; text?: string }>({});
  const current = i18n.resolvedLanguage ?? "id";
  const submit = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const data = new FormData(e.currentTarget);
    const title = String(data.get("title") ?? "").trim();
    const text = String(data.get("text") ?? "");
    const found: typeof errors = {};
    if (title === "") found.title = t("common.required");
    if (text.trim() === "") found.text = t("common.required");
    else if (text.length > importLimits.pasteChars) found.text = t("import.paste_too_long");
    setErrors(found);
    if (Object.keys(found).length === 0) {
      onSend({ format: "paste", language: songLanguages.find((l) => l === data.get("language")), files: [{ name: title, text }] });
    }
  };
  return (
    <section aria-labelledby="import-paste" className="space-y-4 rounded-md border border-border p-4">
      <h2 id="import-paste" className="text-xl font-semibold">{t("import.paste_title")}</h2>
      <p>{t("import.paste_intro")}</p>
      <form noValidate className="space-y-4" onSubmit={submit}>
        <Field label={t("library.title_label")} error={errors.title}><Input name="title" autoComplete="off" /></Field>
        <Field label={t("library.language")}>
          <Select name="language" defaultValue={(songLanguages as readonly string[]).includes(current) ? current : "id"}>
            {songLanguages.map((l) => <option key={l} value={l}>{t(`setup.content_languages.${l}`)}</option>)}
          </Select>
        </Field>
        <Field label={t("import.paste_text")} hint={t("import.paste_hint")} error={errors.text}>
          <textarea name="text" rows={10} className={textarea} />
        </Field>
        <Button type="submit" disabled={busy}>{t("import.paste_submit")}</Button>
      </form>
    </section>
  );
}

function FilesForm({ format, busy, onSend }: {
  format: "openlyrics" | "chordpro";
  busy: boolean;
  onSend: (b: Sent, skipped: ImportRejectedView[]) => void;
}) {
  const { t } = useTranslation();
  const [error, setError] = useState("");
  const [bad, setBad] = useState<ImportRejectedView[]>([]);
  const submit = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    const input = e.currentTarget.elements.namedItem("files") as HTMLInputElement;
    const chosen = Array.from(input.files ?? []);
    setError("");
    setBad([]);
    if (chosen.length === 0) return setError(t("import.files_required"));
    if (chosen.length > importLimits.files) return setError(t("import.files_too_many", { max: importLimits.files }));
    const read = await readFiles(chosen);
    const ok = read.filter((f): f is Extract<PickedFile, { text: string }> => "text" in f);
    const skipped = read.flatMap((f) => ("reason" in f ? [{ name: f.name, song_index: 0, reason: f.reason }] : []));
    if (ok.reduce((n, f) => n + f.text.length, 0) > importLimits.totalBytes) return setError(t("import.files_total"));
    setBad(skipped);
    if (ok.length === 0) return setError(t("import.files_none_readable"));
    onSend({ format, files: ok }, skipped);
  };
  const key = format === "openlyrics" ? "ol" : "cp";
  return (
    <section aria-labelledby={`import-${key}`} className="space-y-4 rounded-md border border-border p-4">
      <h2 id={`import-${key}`} className="text-xl font-semibold">{t(`import.${key}_title`)}</h2>
      <p>{t(`import.${key}_intro`)}</p>
      <form noValidate className="space-y-4" onSubmit={(e) => void submit(e)}>
        <Field label={t(`import.${key}_files`)} hint={t("import.files_hint")} error={error}>
          <Input type="file" name="files" multiple accept={format === "openlyrics" ? ".xml,.txt,text/xml" : ".cho,.crd,.chopro,.chordpro,.pro,.txt,text/plain"} />
        </Field>
        {bad.length > 0 && <Rejected items={bad} />}
        <Button type="submit" disabled={busy}>{t(`import.${key}_submit`)}</Button>
      </form>
    </section>
  );
}

// Rejected lists the files or songs that gave nothing to review, with the reason.
export function Rejected({ items }: { items: ImportRejectedView[] }) {
  const { t } = useTranslation();
  return (
    <Alert variant="error" className="space-y-2">
      <p>{t("import.rejected_title", { count: items.length })}</p>
      <ul className="list-disc pl-6">
        {items.map((r, i) => (
          <li key={i}>{r.name}: {reasonText(t, r.reason)}</li>
        ))}
      </ul>
    </Alert>
  );
}
