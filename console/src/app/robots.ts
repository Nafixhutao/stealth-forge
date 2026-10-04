import type { MetadataRoute } from "next";

export default function robots(): MetadataRoute.Robots {
  return {
    rules: [
      {
        userAgent: "*",
        // The authenticated console is not a public site; only the public
        // status page is intended to be discoverable.
        disallow: "/",
        allow: "/status",
      },
    ],
  };
}
