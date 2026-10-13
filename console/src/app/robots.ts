import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        // The console is authenticated and internal; nothing here is a public
        // site, so keep the whole surface out of search indexes.
        disallow: "/",
      },
    ],
  };
}
