// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

// KeepScreenOn is the "Keep screen on" switch of reading mode (13 §7). It is
// hidden when the browser has no Wake Lock API. The lock is released when the
// page is hidden (the browser does so too) or the switch is turned off, and
// asked for again when the page is shown while the switch is on.
export function KeepScreenOn() {
  const { t } = useTranslation();
  const supported = "wakeLock" in navigator;
  const [on, setOn] = useState(false);
  const [refused, setRefused] = useState(false);
  useEffect(() => {
    if (!on) return;
    let lock: WakeLockSentinel | null = null;
    let stopped = false;
    const acquire = async () => {
      try {
        const next = await navigator.wakeLock.request("screen");
        if (stopped) void next.release();
        else {
          lock = next;
          setRefused(false);
        }
      } catch {
        // For example a low battery: say so, and try again when shown again.
        setRefused(true);
      }
    };
    const onVisibility = () => {
      if (document.visibilityState === "visible") void acquire();
      else {
        void lock?.release();
        lock = null;
      }
    };
    void acquire();
    document.addEventListener("visibilitychange", onVisibility);
    return () => {
      stopped = true;
      document.removeEventListener("visibilitychange", onVisibility);
      void lock?.release();
    };
  }, [on]);
  if (!supported) return null;
  return (
    <div>
      <label className="inline-flex min-h-12 items-center gap-2">
        <input type="checkbox" role="switch" className="size-5" checked={on} onChange={(e) => setOn(e.target.checked)} />
        {t("reading.keep_on")}
      </label>
      {on && refused && <p role="status" className="text-sm">{t("reading.keep_on_refused")}</p>}
    </div>
  );
}
