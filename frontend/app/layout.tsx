import type { Metadata } from "next";
import localFont from "next/font/local";
import "./globals.css";

const inter = localFont({
  src: [
    {
      path: "../design-system/Inter-4.1/web/InterVariable.woff2",
      weight: "100 900",
      style: "normal"
    },
    {
      path: "../design-system/Inter-4.1/web/InterVariable-Italic.woff2",
      weight: "100 900",
      style: "italic"
    }
  ],
  display: "swap",
  variable: "--font-inter"
});

export const metadata: Metadata = {
  title: "Second Brain",
  description: "A source-grounded knowledge inbox for saved links, videos, transcripts, and reading decisions."
};

export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return (
    <html data-scroll-behavior="smooth" lang="en">
      <body className={inter.variable}>{children}</body>
    </html>
  );
}
