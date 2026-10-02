// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";

// ConfirmButton asks before a destructive action, in the page rather than a
// browser dialog: the first press shows the question and "Yes"/"No".
export function ConfirmButton({ label, question, confirmLabel, pending, onConfirm }: {
  label: string;
  question: ReactNode;
  confirmLabel: string;
  pending?: boolean;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  const [asking, setAsking] = useState(false);
  if (!asking) return <Button variant="outline" onClick={() => setAsking(true)}>{label}</Button>;
  return (
    <div role="group" aria-label={label} className="space-y-2 rounded-md border border-destructive p-3">
      <p>{question}</p>
      <div className="flex flex-wrap gap-2">
        <Button variant="destructive" disabled={pending} onClick={onConfirm}>{confirmLabel}</Button>
        <Button variant="outline" onClick={() => setAsking(false)}>{t("common.no")}</Button>
      </div>
    </div>
  );
}
