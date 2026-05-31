"use client";

import { usePathname } from "next/navigation";
import { useEffect, useRef } from "react";

type GoogleAnalyticsProps = {
  measurementId: string;
};

declare global {
  interface Window {
    dataLayer?: unknown[];
    gtag?: (...args: unknown[]) => void;
  }
}

export function GoogleAnalytics({ measurementId }: GoogleAnalyticsProps) {
  const pathname = usePathname();
  const hasTrackedInitialPageView = useRef(false);

  useEffect(() => {
    if (!hasTrackedInitialPageView.current) {
      hasTrackedInitialPageView.current = true;
      return;
    }

    if (!pathname || typeof window.gtag !== "function") {
      return;
    }

    window.gtag("config", measurementId, {
      page_path: `${pathname}${window.location.search}`
    });
  }, [measurementId, pathname]);

  return null;
}
