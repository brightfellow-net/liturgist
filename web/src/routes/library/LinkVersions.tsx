// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState, type FormEvent } from "react";
import { Link } from "react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { SongView } from "@liturgist/api-client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmButton } from "@/components/ConfirmButton";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Field } from "@/components/Field";
import { api, call } from "@/lib/api";
import { songQuery, songsQuery } from "@/lib/library";
import { songPath } from "../paths";

// LinkVersions shows the other-language versions of a song as links, and
// lets an editor link another song or unlink this one (06 §2.3, §4).
export function LinkVersions({ song }: { song: SongView }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const [picking, setPicking] = useState(false);
  const versions = song.versions ?? [];

  // Both songs of a link (or the ones that stay) change their version and
  // their list entries, so everything about songs is fetched again.
  const refresh = async (saved?: SongView) => {
    if (saved) queryClient.setQueryData(songQuery(saved.id).queryKey, saved);
    await queryClient.invalidateQueries({ queryKey: ["song"] });
    await queryClient.invalidateQueries({ queryKey: ["songs"] });
  };
  const unlink = useMutation({
    mutationFn: () => call(api.DELETE("/songs/{id}/link", { params: { path: { id: song.id } } })),
    onSuccess: () => refresh(),
  });

  return (
    <section aria-labelledby="versions-title" className="space-y-3">
      <h2 id="versions-title" className="text-xl font-semibold">{t("library.versions")}</h2>
      {versions.length === 0 ? <p>{t("library.no_versions")}</p> : (
        <ul className="list-disc space-y-1 pl-6">
          {versions.map((v) => (
            <li key={v.id}>
              <Link className="underline" to={songPath(v.id)}>{v.title}</Link>{" "}
              <span className="text-muted-foreground">({t(`setup.content_languages.${v.language}`)})</span>
            </li>
          ))}
        </ul>
      )}
      <ErrorAlert error={unlink.error} />
      {song.actions.edit && (
        <div className="flex flex-wrap gap-2">
          {!picking && <Button variant="outline" onClick={() => setPicking(true)}>{t("library.link_version")}</Button>}
          {versions.length > 0 && (
            <ConfirmButton
              label={t("library.unlink")}
              question={t("library.unlink_question")}
              confirmLabel={t("library.unlink_confirm")}
              pending={unlink.isPending}
              onConfirm={() => unlink.mutate()}
            />
          )}
        </div>
      )}
      {picking && <LinkPicker song={song} onDone={() => setPicking(false)} onLinked={refresh} />}
    </section>
  );
}

function LinkPicker({ song, onDone, onLinked }: { song: SongView; onDone: () => void; onLinked: (saved: SongView) => Promise<void> }) {
  const { t } = useTranslation();
  const [q, setQ] = useState("");
  const found = useQuery({ ...songsQuery({ q, limit: 10 }), enabled: q !== "" });
  const linked = new Set((song.versions ?? []).map((v) => v.id));
  const choices = (found.data?.items ?? []).filter((s) => s.id !== song.id && !linked.has(s.id));
  const link = useMutation({
    mutationFn: (otherId: string) =>
      call(api.POST("/songs/{id}/link", { params: { path: { id: song.id } }, body: { other_song_id: otherId } })),
    onSuccess: async (saved) => {
      await onLinked(saved);
      onDone();
    },
  });
  const search = (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setQ(String(new FormData(e.currentTarget).get("q") ?? "").trim());
  };
  return (
    <div className="space-y-3 rounded-md border border-border p-4">
      <form role="search" className="space-y-3" onSubmit={search}>
        <Field label={t("library.link_search")}>
          <Input type="search" name="q" autoComplete="off" />
        </Field>
        <Button type="submit">{t("library.search")}</Button>
      </form>
      <ErrorAlert error={found.error ?? link.error} />
      {q !== "" && found.data && (choices.length === 0 ? <p role="status">{t("library.link_none")}</p> : (
        <ul className="space-y-2">
          {choices.map((s) => (
            <li key={s.id}>
              <Button variant="outline" disabled={link.isPending} onClick={() => link.mutate(s.id)}>
                {t("library.link_choose", { title: s.title })}
                <span className="font-normal text-muted-foreground"> ({t(`setup.content_languages.${s.language}`)})</span>
              </Button>
            </li>
          ))}
        </ul>
      ))}
      <Button variant="ghost" onClick={onDone}>{t("library.link_close")}</Button>
    </div>
  );
}
