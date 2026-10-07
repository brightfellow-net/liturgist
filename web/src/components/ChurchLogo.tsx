// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from "react";

// ChurchLogo shows the church's logo beside its name (15 §2, L-5). The name
// is always next to it, so the image has an empty alt text. When the file
// cannot be loaded (offline, or lost) it disappears and the name stays; a new
// address (a new logo) is tried again.
export function ChurchLogo({ url, className }: { url?: string | null; className: string }) {
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [url]);
  if (!url || failed) return null;
  return <img src={url} alt="" className={className} onError={() => setFailed(true)} />;
}
