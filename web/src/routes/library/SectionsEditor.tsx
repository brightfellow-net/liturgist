// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { useFieldArray, type UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { Field } from "@/components/Field";
import { limits, sectionKinds, sectionName, type SectionKind } from "@/lib/library";
import { newRef, nextVerseNumber, type SongValues } from "./songForm";

// focusSoon puts the keyboard focus back on a button after React has moved
// its row: moving a node in the page drops the focus.
function focusSoon(ids: string[]) {
  requestAnimationFrame(() => {
    for (const id of ids) {
      const el = document.getElementById(id);
      if (el instanceof HTMLButtonElement && !el.disabled) {
        el.focus();
        return;
      }
    }
  });
}

// SectionsEditor edits the song's sections: one card for each, in order. The
// order is changed with buttons that have text, not by dragging (06 §4, P-52).
export function SectionsEditor({ form }: { form: UseFormReturn<SongValues> }) {
  const { t } = useTranslation();
  const { fields, append, remove, move } = useFieldArray({ control: form.control, name: "sections" });
  const sections = form.watch("sections");
  const errors = form.formState.errors;
  const [announcement, setAnnouncement] = useState("");
  const name = (i: number) => (sections[i] ? sectionName(t, { ...sections[i], number: Number(sections[i].number) || null }) : "");

  const add = () => {
    const kind: SectionKind = sections.length === 0 || sections.every((s) => s.kind === "verse") ? "verse" : "chorus";
    append({ ref: newRef(), kind, number: kind === "verse" ? nextVerseNumber(sections) : "", label: "", text: "" });
  };
  const shift = (i: number, to: number) => {
    const moved = name(i);
    move(i, to);
    setAnnouncement(t("library.moved", { name: moved, n: to + 1, total: fields.length }));
    focusSoon([`${fields[i].id}-${to < i ? "up" : "down"}`, `${fields[i].id}-${to < i ? "down" : "up"}`]);
  };
  const drop = (i: number) => {
    const { ref } = sections[i];
    setAnnouncement(t("library.removed", { name: name(i) }));
    remove(i);
    form.setValue("arrangement", form.getValues("arrangement").filter((r) => r !== ref), { shouldDirty: true });
    focusSoon(["add-section"]);
  };

  return (
    <section aria-labelledby="sections-title" className="space-y-4">
      <div>
        <h2 id="sections-title" className="text-xl font-semibold">{t("library.sections_title")}</h2>
        <p className="text-muted-foreground">{t("library.sections_intro")}</p>
      </div>
      <p role="status" className="sr-only">{announcement}</p>
      {typeof errors.sections?.message === "string" && <p className="text-sm text-destructive">{t(errors.sections.message)}</p>}
      <ol className="space-y-4">
        {fields.map((field, i) => {
          const e = errors.sections?.[i];
          const msg = (m?: string) => (m ? t(m) : undefined);
          const isVerse = sections[i]?.kind === "verse";
          const num = t("library.section_n", { n: i + 1 });
          return (
            <li key={field.id}>
              <fieldset className="space-y-3 rounded-md border border-border p-4">
                <legend className="px-1 font-semibold">{t("library.section_n", { n: i + 1 })}: {name(i)}</legend>
                <div className="grid gap-4 sm:grid-cols-3">
                  <Field label={t("library.section_kind")}>
                    <Select
                      {...form.register(`sections.${i}.kind`, {
                        onChange: (ev) => {
                          if (ev.target.value === "verse" && !form.getValues(`sections.${i}.number`)) {
                            form.setValue(`sections.${i}.number`, nextVerseNumber(form.getValues("sections").filter((_, j) => j !== i)));
                          }
                        },
                      })}
                    >
                      {sectionKinds.map((k) => <option key={k} value={k}>{t(`library.kinds.${k}`)}</option>)}
                    </Select>
                  </Field>
                  {isVerse && (
                    <Field label={t("library.section_number")} error={msg(e?.number?.message)}>
                      <Input inputMode="numeric" autoComplete="off" {...form.register(`sections.${i}.number`)} />
                    </Field>
                  )}
                  <Field label={t("library.section_label")} hint={t("library.section_label_hint")} error={msg(e?.label?.message)}>
                    <Input autoComplete="off" {...form.register(`sections.${i}.label`)} />
                  </Field>
                </div>
                <Field label={t("library.section_text")} error={msg(e?.text?.message)}>
                  <textarea rows={6} className="block min-h-12 w-full rounded-md border border-input bg-background px-3 py-2 text-base aria-invalid:border-destructive" {...form.register(`sections.${i}.text`)} />
                </Field>
                <div className="flex flex-wrap gap-2">
                  <Button id={`${field.id}-up`} aria-label={`${t("library.move_up")} ${num}`} variant="outline" disabled={i === 0} onClick={() => shift(i, i - 1)}>
                    {t("library.move_up")}
                  </Button>
                  <Button id={`${field.id}-down`} aria-label={`${t("library.move_down")} ${num}`} variant="outline" disabled={i === fields.length - 1} onClick={() => shift(i, i + 1)}>
                    {t("library.move_down")}
                  </Button>
                  <Button aria-label={`${t("library.remove")} ${num}`} variant="outline" onClick={() => drop(i)}>
                    {t("library.remove")}
                  </Button>
                </div>
              </fieldset>
            </li>
          );
        })}
      </ol>
      {fields.length === 0 && <p>{t("library.no_sections")}</p>}
      <Button id="add-section" variant="outline" disabled={fields.length >= limits.sections} onClick={add}>
        {t("library.add_section")}
      </Button>
    </section>
  );
}
