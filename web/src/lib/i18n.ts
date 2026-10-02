// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import en from "@liturgist/i18n/en.json";
import id from "@liturgist/i18n/id.json";
import type { Me } from "@liturgist/api-client";

export type Language = "en" | "id";
export const languages: readonly Language[] = ["en", "id"];

// browserLanguage is the language before login: Indonesian if the browser
// asks for it, else English (05 §6).
export function browserLanguage(navigatorLanguage: string): Language {
  return navigatorLanguage.toLowerCase().startsWith("id") ? "id" : "en";
}

// chooseLanguage picks the logged-in user's language: their preference, then
// the church default, then English (05 §6).
export function chooseLanguage(me: Pick<Me, "user" | "church">): Language {
  const pick = me.user.preferences.ui_language ?? me.church?.default_ui_language;
  return pick === "id" ? "id" : "en";
}

void i18n.use(initReactI18next).init({
  resources: { en: { translation: en }, id: { translation: id } },
  lng: browserLanguage(typeof navigator === "undefined" ? "en" : navigator.language),
  fallbackLng: "en",
  interpolation: { escapeValue: false }, // React escapes
});

i18n.on("languageChanged", (lng) => {
  document.documentElement.lang = lng;
});
document.documentElement.lang = i18n.language;

export default i18n;
