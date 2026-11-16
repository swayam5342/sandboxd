import type { Metadata, Viewport } from "next";
import { Instrument_Sans } from "next/font/google";
import "./globals.css";

const instrument = Instrument_Sans({ subsets: ["latin"], variable: "--font-instrument", display: "swap" });

const base = process.env.NEXT_PUBLIC_BASE_PATH ?? "";

export const metadata: Metadata = {
  metadataBase: new URL("https://sandboxd.swayam.li"),
  title: "sandboxd: run code you don’t trust",
  description:
    "A Go daemon that compiles and runs untrusted code in any configured language, jailed by NsJail as user nobody, offline, behind a JSON API.",
  openGraph: {
    title: "sandboxd: run code you don’t trust",
    description: "A Go daemon that runs untrusted code in any configured language inside NsJail, as uid 65534, offline, behind a JSON API.",
    images: [{ url: `${base}/og.png`, width: 1200, height: 630, alt: "sandboxd: run code you don’t trust" }],
    type: "website",
    url: "https://sandboxd.swayam.li/",
  },
  twitter: { card: "summary_large_image", title: "sandboxd: run code you don’t trust", images: [`${base}/og.png`] },
  icons: {
    icon: [
      { url: `${base}/icon.svg`, type: "image/svg+xml" },
      { url: `${base}/favicon-32.png`, sizes: "32x32", type: "image/png" },
    ],
    apple: `${base}/apple-touch-icon.png`,
  },
  manifest: `${base}/manifest.webmanifest`,
  alternates: { canonical: "https://sandboxd.swayam.li/" },
};

export const viewport: Viewport = { themeColor: "#000000", width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={instrument.variable} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: "document.documentElement.classList.add('js')" }} />
      </head>
      <body>{children}</body>
    </html>
  );
}
