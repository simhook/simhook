import type { Metadata } from "next";
import { Geist_Mono, Instrument_Sans } from "next/font/google";
import { Providers } from "@/components/providers";
import "./globals.css";

const sans = Instrument_Sans({ variable: "--font-instrument-sans", subsets: ["latin"] });
const mono = Geist_Mono({ variable: "--font-geist-mono", subsets: ["latin"] });

export const metadata: Metadata = {
  // Only used to make the share image's address absolute; the dashboard is not for search engines.
  metadataBase: new URL(process.env.NEXT_PUBLIC_APP_URL ?? "https://app.simhook.dev"),
  title: { default: "simhook", template: "%s · simhook" },
  description: "The simhook dashboard: phones, messages, webhooks, and API keys.",
  robots: { index: false, follow: false },
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable} h-full antialiased`} suppressHydrationWarning>
      <body className="min-h-full flex flex-col font-sans">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
