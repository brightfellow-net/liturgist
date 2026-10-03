// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { Link, useNavigate, useParams } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { SongView } from "@liturgist/api-client";
import { Alert } from "@/components/ui/alert";
import { buttonVariants } from "@/components/ui/button";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { api, call } from "@/lib/api";
import { isCode } from "@/lib/errors";
import { hymnalText, sectionName, songQuery } from "@/lib/library";
import { paths, songEditPath } from "../paths";
import { LinkVersions } from "./LinkVersions";

// SongPage shows one song: details, credit line, the lyrics section by
// section, the default order and the other-language versions (06 §4).
export function SongPage() {
  const { id = "" } = useParams();
  const { t } = useTranslation();
  const song = useQuery(songQuery(id));
  if (song.error) {
    return isCode(song.error, "not_found") ? (
      <div className="space-y-4">
        <Alert variant="error">{t("library.not_found")}</Alert>
        <Link className="underline" to={paths.library}>{t("library.back")}</Link>
      </div>
    ) : <ErrorAlert error={song.error} onRetry={() => void song.refetch()} />;
  }
  if (!song.data) return <p role="status">{t("app.loading")}</p>;
  return <SongDetails song={song.data} />;
}

function SongDetails({ song }: { song: SongView }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const sections = song.sections ?? [];
  const names = new Map(sections.map((s) => [s.id, sectionName(t, s)]));
  const arrangement = song.default_arrangement ?? [];
  const remove = useMutation({
    mutationFn: () => call(api.DELETE("/songs/{id}", { params: { path: { id: song.id } } })),
    onSuccess: async () => {
      queryClient.removeQueries({ queryKey: ["song", song.id] });
      await queryClient.invalidateQueries({ queryKey: ["songs"] });
      void navigate(paths.library);
    },
  });
  const details: [string, string][] = [
    [t("library.language"), t(`setup.content_languages.${song.language}`)],
    [t("library.hymnal"), hymnalText(song)],
    [t("library.default_key"), song.default_key],
    [t("library.lyricist"), song.lyricist],
    [t("library.composer"), song.composer],
    [t("library.translator"), song.translator],
    [t("library.copyright_holder"), song.copyright_holder],
    [t("library.ccli"), song.ccli_song_number],
    [t("library.licence_status"), t(`library.licence.${song.licence_status}`)],
    [t("library.licence_notes"), song.licence_notes],
  ];
  return (
    <article className="space-y-8">
      <p><Link className="underline" to={paths.library}>{t("library.back")}</Link></p>
      <header className="space-y-3">
        <h1 className="text-2xl font-semibold">{song.title}</h1>
        {(song.alt_titles ?? []).length > 0 && (
          <p className="text-muted-foreground">{t("library.also_known_as")}: {(song.alt_titles ?? []).join(" · ")}</p>
        )}
        {song.copyright_line && <p>{song.copyright_line}</p>}
        <ErrorAlert error={remove.error} />
        {(song.actions.edit || song.actions.delete) && (
          <div className="flex flex-wrap gap-2">
            {song.actions.edit && <Link className={buttonVariants({ variant: "outline" })} to={songEditPath(song.id)}>{t("library.edit")}</Link>}
            {song.actions.delete && (
              <ConfirmButton
                label={t("library.delete")}
                question={t("library.delete_question", { title: song.title })}
                confirmLabel={t("library.delete_confirm", { title: song.title })}
                pending={remove.isPending}
                onConfirm={() => remove.mutate()}
              />
            )}
          </div>
        )}
      </header>

      <dl className="grid gap-x-6 gap-y-2 sm:grid-cols-[max-content_1fr]">
        {details.filter(([, value]) => value !== "").map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="font-medium">{label}</dt>
            <dd className="whitespace-pre-line">{value}</dd>
          </div>
        ))}
      </dl>

      <section aria-labelledby="lyrics-title" className="space-y-4">
        <h2 id="lyrics-title" className="text-xl font-semibold">{t("library.lyrics")}</h2>
        {sections.length === 0 && <p>{t("library.no_sections")}</p>}
        {sections.map((s) => (
          <div key={s.id} className="space-y-1">
            <h3 className="font-semibold">{names.get(s.id)}</h3>
            <p className="whitespace-pre-line">{s.text}</p>
          </div>
        ))}
      </section>

      <section aria-labelledby="order-title" className="space-y-2">
        <h2 id="order-title" className="text-xl font-semibold">{t("library.arrangement")}</h2>
        {arrangement.length === 0 ? <p>{t("library.arrangement_all")}</p> : (
          <ol className="list-decimal space-y-1 pl-8">
            {arrangement.map((ref, i) => <li key={i}>{names.get(ref)}</li>)}
          </ol>
        )}
      </section>

      <LinkVersions song={song} />
    </article>
  );
}
