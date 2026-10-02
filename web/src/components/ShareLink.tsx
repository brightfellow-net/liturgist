// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

// ShareLink shows an invite or reset link as received from the API, with
// "Copy" and "Share to WhatsApp" (05 §5). The text field is the fallback
// when the clipboard can't be used.
export function ShareLink({ label, link, message, expires }: {
  label: string;
  link: string;
  message: string; // the WhatsApp text, including the link
  expires: string;
}) {
  const { t } = useTranslation();
  const input = useRef<HTMLInputElement>(null);
  const [copied, setCopied] = useState<"yes" | "failed" | null>(null);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(link);
      setCopied("yes");
    } catch {
      input.current?.select();
      setCopied("failed");
    }
  };
  return (
    <div className="space-y-2 rounded-md border border-border p-4">
      <Label htmlFor="share-link">{label}</Label>
      <Input id="share-link" ref={input} readOnly value={link} onFocus={(e) => e.target.select()} />
      <p className="text-sm text-muted-foreground">{expires}</p>
      <div className="flex flex-wrap gap-2">
        <Button variant="outline" onClick={() => void copy()}>{t("share.copy")}</Button>
        <a
          className="inline-flex min-h-12 items-center rounded-md border border-border px-4 hover:bg-muted"
          href={"https://wa.me/?text=" + encodeURIComponent(message)}
          target="_blank"
          rel="noreferrer"
        >
          {t("share.whatsapp")}
        </a>
      </div>
      <p role="status" className="text-sm">
        {copied === "yes" && t("share.copied")}
        {copied === "failed" && t("share.copy_failed")}
      </p>
    </div>
  );
}
