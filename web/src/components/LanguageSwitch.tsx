// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useTranslation } from "react-i18next";
import { Select } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { languages } from "@/lib/i18n";

// LanguageSwitch is shown on pages used before login (05 §6).
export function LanguageSwitch() {
  const { t, i18n } = useTranslation();
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor="language-switch">{t("common.language")}</Label>
      <Select id="language-switch" className="w-auto" value={i18n.language} onChange={(e) => void i18n.changeLanguage(e.target.value)}>
        {languages.map((l) => (
          <option key={l} value={l}>{t(`language.${l}`)}</option>
        ))}
      </Select>
    </div>
  );
}
