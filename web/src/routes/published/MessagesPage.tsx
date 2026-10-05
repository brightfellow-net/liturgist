// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useRef, useState } from "react";
import { Link, useOutletContext, useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { Me } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { Button, buttonVariants } from "@/components/ui/button";
import { ErrorAlert } from "@/components/ErrorAlert";
import { isCode } from "@/lib/errors";
import {
  changeText, defaultMessageOptions, isLocalLink, messageLanguage, messageT, personalText, teamText, whatsappLink,
  type Composed, type MessageOptions,
} from "@/lib/messages";
import { summaryQuery } from "@/lib/published";
import { publishedPath } from "../paths";

// MessagesPage offers the three WhatsApp texts of a published liturgy (13 §8).
// The app sends nothing: Copy, or a wa.me link that opens WhatsApp with the
// text filled in for the sender to send.
export function MessagesPage() {
  const { t } = useTranslation();
  const me = useOutletContext<Me>();
  const { id = "" } = useParams();
  const summary = useQuery(summaryQuery(id));
  const [opts, setOpts] = useState<MessageOptions>(defaultMessageOptions);

  if (summary.isPending) return <p role="status">{t("app.loading")}</p>;
  if (isCode(summary.error, "not_found")) return <Alert>{t("published.not_published")}</Alert>;
  if (summary.error || !summary.data) return <ErrorAlert error={summary.error} onRetry={() => void summary.refetch()} />;
  const s = summary.data;
  const lang = messageLanguage(s.liturgy.language, me.church?.default_ui_language);
  const mt = messageT(lang);
  const changes = changeText(s, opts, mt, lang);
  const toggle = (k: keyof MessageOptions) => setOpts((o) => ({ ...o, [k]: !o[k] }));
  return (
    <div className="max-w-3xl space-y-6">
      <div>
        <Link className="underline" to={publishedPath(id)}>{t("messages.back")}</Link>
      </div>
      <h1 className="text-2xl font-semibold">{t("messages.title")}</h1>
      <p>{t("messages.intro", { number: s.number })}</p>
      {isLocalLink(s.url) && <Alert>{t("messages.local_link")}</Alert>}
      <fieldset className="flex flex-wrap items-center gap-4">
        <legend className="mb-1 font-medium">{t("messages.include")}</legend>
        {(["songs", "keys", "readings"] as const).map((k) => (
          <label key={k} className="inline-flex min-h-12 items-center gap-2">
            <input type="checkbox" className="size-5" checked={opts[k]} onChange={() => toggle(k)} />
            {t(`messages.${k}`)}
          </label>
        ))}
      </fieldset>

      <section aria-labelledby="team-title" className="space-y-2">
        <h2 id="team-title" className="text-xl font-semibold">{t("messages.team_title")}</h2>
        <p className="text-muted-foreground">{t("messages.team_note")}</p>
        <Message what={t("messages.team_title")} composed={teamText(s, opts, mt, lang)} share />
      </section>

      <section aria-labelledby="people-title" className="space-y-3">
        <h2 id="people-title" className="text-xl font-semibold">{t("messages.people_title")}</h2>
        <ul className="space-y-4">
          {s.recipients.map((p, i) => (
            <li key={`${p.name}-${i}`} className="space-y-2 rounded-md border border-border p-4">
              <h3 className="font-medium">{p.name} <span className="font-normal text-muted-foreground">· {p.duties.join(", ")}</span></h3>
              {p.member ? (
                <Message what={p.name} composed={personalText(s, p, opts, mt, lang)} phone={p.phone} name={p.name} />
              ) : (
                <p className="text-sm text-muted-foreground">{t("messages.free_text")}</p>
              )}
            </li>
          ))}
        </ul>
      </section>

      <section aria-labelledby="changes-title" className="space-y-2">
        <h2 id="changes-title" className="text-xl font-semibold">{t("messages.changes_title")}</h2>
        {!s.changes ? <p>{t("messages.first_version")}</p> : changes ? <Message what={t("messages.changes_title")} composed={changes} share /> : <p>{t("messages.no_changes")}</p>}
      </section>
    </div>
  );
}

// Message shows one text before its buttons. A text with a phone number gets a
// WhatsApp link to that chat; "share" opens the chat picker; every text can be
// copied, with the text selected for copying by hand when the browser refuses.
function Message({ what, composed, phone, name, share }: { what: string; composed: Composed; phone?: string; name?: string; share?: boolean }) {
  const { t } = useTranslation();
  const area = useRef<HTMLTextAreaElement>(null);
  const [copy, setCopy] = useState<"idle" | "done" | "failed">("idle");
  const doCopy = async () => {
    try {
      await navigator.clipboard.writeText(composed.text);
      setCopy("done");
    } catch {
      area.current?.focus();
      area.current?.select();
      setCopy("failed");
    }
  };
  return (
    <div className="space-y-2">
      <textarea
        ref={area}
        readOnly
        aria-label={t("messages.preview", { what })}
        value={composed.text}
        rows={Math.min(16, composed.text.split("\n").length + 1)}
        className="w-full rounded-md border border-input bg-background p-2 font-mono text-sm"
        onChange={() => undefined}
      />
      {composed.long && <Alert>{t("messages.long")}</Alert>}
      {composed.trimmed && !composed.long && <p className="text-sm text-muted-foreground">{t("messages.trimmed")}</p>}
      <div className="flex flex-wrap items-center gap-2">
        <Button variant="outline" onClick={() => void doCopy()}>{name ? t("messages.copy_for", { name }) : t("messages.copy")}</Button>
        {phone ? (
          <a className={buttonVariants({ variant: "default" })} href={whatsappLink(composed.text, phone)} target="_blank" rel="noopener noreferrer">
            {t("messages.send", { name })}
          </a>
        ) : share ? (
          <a className={buttonVariants({ variant: "default" })} href={whatsappLink(composed.text)} target="_blank" rel="noopener noreferrer">
            {t("messages.share")}
          </a>
        ) : (
          <span className="text-sm text-muted-foreground">{t("messages.no_phone")}</span>
        )}
        <span role="status" className="text-sm">
          {copy === "done" && t("messages.copied")}
          {copy === "failed" && t("messages.copy_failed")}
        </span>
      </div>
    </div>
  );
}
