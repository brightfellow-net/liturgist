// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ErrorAlert } from "@/components/ErrorAlert";
import { editsQuery, editText } from "@/lib/liturgy";

// History lists the last changes in words (11 §4). Slice 3D adds Undo and
// Redo here.
export function History({ id }: { id: string }) {
  const { t, i18n } = useTranslation();
  const edits = useQuery(editsQuery(id));
  const items = edits.data?.items ?? [];
  const when = new Intl.DateTimeFormat(i18n.language, { dateStyle: "medium", timeStyle: "short" });
  return (
    <section aria-labelledby="history-title" className="space-y-2">
      <h3 id="history-title" className="text-lg font-semibold">{t("liturgy.history.title")}</h3>
      <ErrorAlert error={edits.error} onRetry={() => void edits.refetch()} />
      {edits.isPending && <p role="status">{t("app.loading")}</p>}
      {edits.data && items.length === 0 && <p className="text-muted-foreground">{t("liturgy.history.none")}</p>}
      <ul className="space-y-1">
        {items.map((e) => (
          <li key={e.id}>
            {editText(t, e)} <span className="text-sm text-muted-foreground">· {when.format(new Date(e.created_at))}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}
