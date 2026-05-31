import type { Metadata } from "next";
import localFont from "next/font/local";
import { GoogleAnalytics } from "./GoogleAnalytics";
import "./globals.css";

const inter = localFont({
  src: [
    {
      path: "../design-system/Inter-4.1/web/InterVariable.woff2",
      weight: "100 900",
      style: "normal"
    }
  ],
  display: "swap",
  variable: "--font-inter"
});

const googleAnalyticsMeasurementId = process.env.NEXT_PUBLIC_GA_MEASUREMENT_ID;
const googleAnalyticsScript = googleAnalyticsMeasurementId
  ? `
      window.dataLayer = window.dataLayer || [];
      function gtag(){window.dataLayer.push(arguments);}
      window.gtag = gtag;
      window.gtag('js', new Date());
      window.gtag('config', ${JSON.stringify(googleAnalyticsMeasurementId)}, {
        page_path: window.location.pathname + window.location.search
      });
    `
  : undefined;

export const metadata: Metadata = {
  metadataBase: new URL("https://abhijitmohanty.com"),
  applicationName: "Second Brain",
  title: "Second Brain | Knowledge Inbox",
  description: "Source-grounded insights from saved X bookmarks, YouTube videos, transcripts, and daily digests.",
  alternates: {
    canonical: "/second-brain/"
  },
  icons: {
    icon: [
      { url: "/second-brain/favicon.svg", type: "image/svg+xml" },
      { url: "/second-brain/favicon-32.png", sizes: "32x32", type: "image/png" }
    ],
    apple: [{ url: "/second-brain/apple-touch-icon.png", sizes: "180x180", type: "image/png" }]
  },
  openGraph: {
    title: "Second Brain | Knowledge Inbox",
    description: "Source-grounded insights from saved X bookmarks, YouTube videos, transcripts, and daily digests.",
    url: "/second-brain/",
    siteName: "Abhijit Mohanty",
    images: [
      {
        url: "/second-brain/og-image.png",
        width: 1200,
        height: 630,
        alt: "Second Brain Knowledge Inbox"
      }
    ],
    locale: "en_US",
    type: "website"
  },
  twitter: {
    card: "summary_large_image",
    title: "Second Brain | Knowledge Inbox",
    description: "Source-grounded insights from saved X bookmarks, YouTube videos, transcripts, and daily digests.",
    images: ["/second-brain/og-image.png"]
  }
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html data-scroll-behavior="smooth" lang="en">
      {googleAnalyticsMeasurementId && googleAnalyticsScript ? (
        <head>
          <script async src={`https://www.googletagmanager.com/gtag/js?id=${googleAnalyticsMeasurementId}`} />
          <script dangerouslySetInnerHTML={{ __html: googleAnalyticsScript }} />
        </head>
      ) : null}
      <body className={inter.variable}>
        {googleAnalyticsMeasurementId ? <GoogleAnalytics measurementId={googleAnalyticsMeasurementId} /> : null}
        {children}
      </body>
    </html>
  );
}
