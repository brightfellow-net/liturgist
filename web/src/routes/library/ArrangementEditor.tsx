// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import type { UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Select } from "@/components/ui/input";
import { Field } from "@/components/Field";
import { limits, sectionName } from "@/lib/library";
import type { SongValues } from "./songForm";

// ArrangementEditor edits the default order of the sections, for example
// verse 1, chorus, verse 2, chorus. Empty means "all sections in order".
export function ArrangementEditor({ form }: { form: UseFormReturn<SongValues> }) {
  const { t } = useTranslation();
  const sections = form.watch("sections");
  const arrangement = form.watch("arrangement");
  const [pick, setPick] = useState("");
  const names = new Map(sections.map((s) => [s.ref, sectionName(t, { ...s, number: Number(s.number) || null })]));
  const set = (next: string[]) => form.setValue("arrangement", next, { shouldDirty: true, shouldValidate: true });
  const swap = (i: number, j: number) => {
    const next = [...arrangement];
    [next[i], next[j]] = [next[j], next[i]];
    set(next);
  };
  const chosen = pick && names.has(pick) ? pick : sections[0]?.ref ?? "";
  const full = arrangement.length >= limits.arrangement;
  const error = form.formState.errors.arrangement?.message;

  return (
    <section aria-labelledby="arrangement-title" className="space-y-4">
      <div>
        <h2 id="arrangement-title" className="text-xl font-semibold">{t("library.arrangement_title")}</h2>
        <p className="text-muted-foreground">{t("library.arrangement_hint")}</p>
      </div>
      {arrangement.length === 0 ? <p>{t("library.arrangement_empty")}</p> : (
        <ol className="list-decimal space-y-2 pl-8">
          {arrangement.map((ref, i) => (
            <li key={i} className="pl-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="min-w-24 font-medium">{names.get(ref)}</span>
                <Button aria-label={`${t("library.move_up")} ${names.get(ref)} ${i + 1}`} variant="outline" disabled={i === 0} onClick={() => swap(i, i - 1)}>
                  {t("library.move_up")}
                </Button>
                <Button aria-label={`${t("library.move_down")} ${names.get(ref)} ${i + 1}`} variant="outline" disabled={i === arrangement.length - 1} onClick={() => swap(i, i + 1)}>
                  {t("library.move_down")}
                </Button>
                <Button aria-label={`${t("library.remove")} ${names.get(ref)} ${i + 1}`} variant="outline" onClick={() => set(arrangement.filter((_, j) => j !== i))}>
                  {t("library.remove")}
                </Button>
              </div>
            </li>
          ))}
        </ol>
      )}
      {typeof error === "string" && <p className="text-sm text-destructive">{t(error)}</p>}
      {sections.length > 0 && (
        <div className="flex flex-wrap items-end gap-2">
          <Field label={t("library.arrangement_pick")}>
            <Select value={chosen} onChange={(e) => setPick(e.target.value)}>
              {sections.map((s, i) => <option key={s.ref} value={s.ref}>{i + 1}. {names.get(s.ref)}</option>)}
            </Select>
          </Field>
          <Button variant="outline" disabled={full || !chosen} onClick={() => set([...arrangement, chosen])}>
            {t("library.arrangement_add")}
          </Button>
          {arrangement.length > 0 && (
            <Button variant="outline" onClick={() => set([])}>{t("library.arrangement_clear")}</Button>
          )}
        </div>
      )}
    </section>
  );
}
