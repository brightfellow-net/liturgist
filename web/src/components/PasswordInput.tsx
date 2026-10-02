// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useState, type ComponentProps } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

// PasswordInput has a "Show password" button with a text label (05 §7).
export function PasswordInput(props: Omit<ComponentProps<"input">, "type">) {
  const { t } = useTranslation();
  const [shown, setShown] = useState(false);
  return (
    <div className="flex gap-2">
      <Input type={shown ? "text" : "password"} className="flex-1" {...props} />
      <Button variant="outline" aria-pressed={shown} onClick={() => setShown(!shown)}>
        {shown ? t("common.hide_password") : t("common.show_password")}
      </Button>
    </div>
  );
}
