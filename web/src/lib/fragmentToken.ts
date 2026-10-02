// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from "react";

// readFragmentToken returns the token in "#t=…" and removes the fragment
// from the address bar and history (05 §5), or null if there is none.
export function readFragmentToken(): string | null {
  const params = new URLSearchParams(window.location.hash.slice(1));
  const token = params.get("t");
  if (window.location.hash !== "") {
    const { pathname, search } = window.location;
    window.history.replaceState(window.history.state, "", pathname + search);
  }
  return token ? token : null;
}

// useFragmentToken reads the token once when the page opens. A link opened
// while the same page is showing changes only the fragment, so the page is
// loaded again to start over with the new token.
export function useFragmentToken(): string | null {
  const [token] = useState(readFragmentToken);
  useEffect(() => {
    const onChange = () => {
      if (new URLSearchParams(window.location.hash.slice(1)).has("t")) window.location.reload();
    };
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);
  return token;
}
