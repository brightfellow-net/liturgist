// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState } from "react";
import { Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field } from "@/components/Field";
import { ErrorAlert } from "@/components/ErrorAlert";
import { hymnalText, songsQuery } from "@/lib/library";
import { readingsQuery } from "@/lib/readings";
import { useDebounced } from "@/lib/useDebounced";
import { paths } from "../paths";
import type { ReadingChoice } from "./itemDraft";

// SongPicker searches the library and adds the chosen song to the item; the
// server fills its sequence (11 §4).
export function SongPicker({ onPick, disabled, pending }: { onPick: (songId: string) => void; disabled?: boolean; pending?: boolean }) {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const q = useDebounced(text.trim(), 250);
  const results = useQuery({ ...songsQuery({ q, limit: 8 }), enabled: q !== "" });
  return (
    <div className="space-y-2 rounded-md border border-border p-3">
      <Field label={t("liturgy.song.search")} hint={t("liturgy.song.search_hint")}>
        <Input autoComplete="off" value={text} disabled={disabled} onChange={(e) => setText(e.target.value)} />
      </Field>
      <ErrorAlert error={results.error} onRetry={() => void results.refetch()} />
      {q !== "" && results.data && (results.data.items ?? []).length === 0 && (
        <p>{t("liturgy.song.no_results")} <Link className="underline" to={paths.songNew}>{t("liturgy.song.add_new")}</Link></p>
      )}
      <ul className="space-y-2">
        {(results.data?.items ?? []).map((s) => (
          <li key={s.id} className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border p-2">
            <span>
              <span className="font-medium">{s.title}</span>
              <span className="block text-sm text-muted-foreground">
                {[hymnalText(s), t(`setup.content_languages.${s.language}`)].filter(Boolean).join(" · ")}
              </span>
            </span>
            <Button variant="outline" disabled={disabled || pending} aria-label={`${t("liturgy.song.add")} ${s.title}`} onClick={() => onPick(s.id)}>
              {t("liturgy.song.add")}
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}

// ReadingPicker searches the saved readings (11 §4).
export function ReadingPicker({ onPick }: { onPick: (r: ReadingChoice) => void }) {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const q = useDebounced(text.trim(), 250);
  const results = useQuery({ ...readingsQuery({ q, limit: 8 }), enabled: q !== "" });
  return (
    <div className="space-y-2 rounded-md border border-border p-3">
      <Field label={t("liturgy.reading.search")} hint={t("liturgy.reading.search_hint")}>
        <Input autoComplete="off" value={text} onChange={(e) => setText(e.target.value)} />
      </Field>
      <ErrorAlert error={results.error} onRetry={() => void results.refetch()} />
      {q !== "" && results.data && (results.data.items ?? []).length === 0 && (
        <p>{t("liturgy.reading.no_results")} <Link className="underline" to={paths.readingNew}>{t("liturgy.reading.add_new")}</Link></p>
      )}
      <ul className="space-y-2">
        {(results.data?.items ?? []).map((r) => (
          <li key={r.id} className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border p-2">
            <span>
              <span className="font-medium">{r.canonical}</span>
              <span className="block text-sm text-muted-foreground">{r.translation.code} · {r.snippet}</span>
            </span>
            <Button
              variant="outline"
              aria-label={`${t("liturgy.reading.choose_this")} ${r.canonical}`}
              onClick={() => { onPick({ id: r.id, label: `${r.canonical} (${r.translation.code})`, text: r.snippet }); setText(""); }}
            >
              {t("liturgy.reading.choose_this")}
            </Button>
          </li>
        ))}
      </ul>
    </div>
  );
}
