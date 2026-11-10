import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRootRoute, HeadContent, Outlet, Scripts, useRouterState } from "@tanstack/react-router";
import { MotionConfig } from "motion/react";
import { useState, type ReactNode } from "react";
import type { ContentModule } from "../../buildtime/schema";
import { HeadTags, type PageHead } from "../i18n/head";
import { WordsProvider } from "../i18n/content";
import { language } from "../lib/site";
import "../styles.css";

// PageData is what every page's loader returns: the page's head and its language's words.
export interface PageData {
  head: PageHead;
  content: ContentModule;
}

export const Route = createRootRoute({
  shellComponent: RootDocument,
  component: () => <Outlet />,
});

function RootDocument({ children }: { children: ReactNode }) {
  const page = useRouterState({ select: (s) => s.matches[s.matches.length - 1]?.loaderData as PageData | undefined });
  const [queryClient] = useState(() => new QueryClient({ defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } } }));
  if (!page) {
    throw new Error("the page has no data");
  }
  const lang = language(page.head.code);
  const root = page.head.kind === "home" && page.head.code === "en";
  return (
    <html lang={lang.tag} data-root={root ? "" : undefined} suppressHydrationWarning>
      <head>
        <meta charSet="utf-8" />
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <meta name="color-scheme" content="light" />
        <meta name="theme-color" content="#fbf8f3" />
        <link rel="icon" href="/favicon.ico" sizes="32x32" />
        <link rel="icon" href="/favicon-16.png" sizes="16x16" type="image/png" />
        <link rel="apple-touch-icon" href="/apple-touch-icon.png" />
        <HeadContent />
        <HeadTags page={page.head} />
      </head>
      <body>
        <QueryClientProvider client={queryClient}>
          <MotionConfig reducedMotion="user">
            <WordsProvider content={page.content}>{children}</WordsProvider>
          </MotionConfig>
        </QueryClientProvider>
        <Scripts />
      </body>
    </html>
  );
}
