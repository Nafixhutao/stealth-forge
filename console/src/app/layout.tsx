import type { Metadata, Viewport } from "next";
import "./globals.css";
import { Providers } from "./providers";
import { APP_NAME } from "@/lib/constants";

export const metadata: Metadata = {
  title: {
    default: APP_NAME,
    template: `%s · ${APP_NAME}`,
  },
  description: "Developer operating console for Stealth cloud projects.",
  // The console is authenticated; only the public status page opts back in.
  robots: { index: false, follow: false },
};

export const viewport: Viewport = {
  colorScheme: "dark",
  themeColor: "#171718",
  // Let the app paint under the notch/home indicator; the shell pads content
  // back out with env(safe-area-inset-*). resizes-content keeps 100dvh honest
  // when the Android software keyboard opens.
  viewportFit: "cover",
  interactiveWidget: "resizes-content",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" data-scroll-behavior="smooth">
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
