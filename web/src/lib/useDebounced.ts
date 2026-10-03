// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { useEffect, useState } from "react";

// useDebounced returns value once it has stopped changing for ms
// milliseconds, so typing does not send a request for every key.
export function useDebounced<T>(value: T, ms = 400): T {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), ms);
    return () => clearTimeout(timer);
  }, [value, ms]);
  return settled;
}
