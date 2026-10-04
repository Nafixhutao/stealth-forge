import type { Metadata } from "next";

// The status page is a client component, so this layout owns its public
// metadata and opts the route back into indexing.
export const metadata: Metadata = {
  title: "Status",
  robots: { index: true, follow: true },
};

export default function StatusLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return children;
}
