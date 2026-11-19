// A language's privacy policy: what Kando's desktop and phone apps and this website send, to whom and when, for the stores' privacy policy links and for anyone who asks. Its text is the content files' privacy… strings; the address questions go to is privacyContact.
import type { ReactNode } from "react";
import { Rich, useWords, type ContentKey } from "../../i18n/content";
import { privacyContact } from "../../lib/site";
import { Footer } from "../footer/Footer";
import { Header } from "../header/Header";

// sections are the policy's sections in order, each its heading's content key and its body's.
const sections: readonly [ContentKey, ContentKey][] = [
  ["privacyWhoTitle", "privacyWhoBody"],
  ["privacyCollectTitle", "privacyCollectBody"],
  ["privacyPhoneTitle", "privacyPhoneBody"],
  ["privacyDesktopTitle", "privacyDesktopBody"],
  ["privacyFeedbackTitle", "privacyFeedbackBody"],
  ["privacyOthersTitle", "privacyOthersBody"],
  ["privacySiteTitle", "privacySiteBody"],
  ["privacyChoicesTitle", "privacyChoicesBody"],
  ["privacyChildrenTitle", "privacyChildrenBody"],
  ["privacyChangesTitle", "privacyChangesBody"],
  ["privacyContactTitle", "privacyContactBody"],
];

const paragraphClass = "mt-3 text-[16px] leading-[1.8] text-ink-2";

// Body is a section's text: each line of the content string a paragraph, and a run of lines that start with "- " a list.
function Body({ text }: { text: string }) {
  const out: ReactNode[] = [];
  let items: string[] = [];
  const flush = () => {
    if (items.length > 0) {
      out.push(
        <ul key={out.length} className="mt-3 list-disc space-y-2 pl-6 text-[16px] leading-[1.8] text-ink-2 marker:text-ink-3">
          {items.map((item, i) => (
            <li key={i}>
              <Rich text={item} />
            </li>
          ))}
        </ul>,
      );
      items = [];
    }
  };
  for (const line of text.split("\n")) {
    if (line.startsWith("- ")) {
      items.push(line.slice(2));
      continue;
    }
    flush();
    out.push(
      <p key={out.length} className={paragraphClass}>
        <Rich text={line} />
      </p>,
    );
  }
  flush();
  return <>{out}</>;
}

export function PrivacyPage() {
  const { t } = useWords();
  const contact = privacyContact.replace(/^(https:\/\/|mailto:)/, "");
  return (
    <>
      <Header page="privacy" />
      <main id="main" className="relative pt-[132px] pb-[100px] phone:pt-[108px]">
        <div className="pointer-events-none absolute inset-x-0 top-0 h-[520px] overflow-hidden" aria-hidden="true">
          <span className="blob blob-a" />
          <span className="blob blob-b" />
        </div>
        <div className="wrap relative">
          <header className="head mx-auto max-w-[720px]">
            <h1 className="text-[clamp(34px,4.6vw,56px)] leading-[1.12] font-extrabold tracking-[-0.03em]">{t("privacyPageTitle")}</h1>
            <p className="mt-3 text-[14px] text-ink-3">{t("privacyUpdated")}</p>
            <p className="mx-auto mt-[18px] max-w-[640px] text-[17px] leading-[1.75] text-ink-2">{t("privacyLead")}</p>
          </header>
          <article className="card mx-auto mt-10 max-w-[760px] rounded-[22px] px-10 py-9 phone:px-6 phone:py-7">
            {sections.map(([title, body], i) => (
              <section key={title} className={i === 0 ? undefined : "mt-9"}>
                <h2 className="text-[20px] leading-[1.35] font-bold tracking-[-0.01em]">{t(title)}</h2>
                <Body text={t(body)} />
              </section>
            ))}
            <p className={paragraphClass}>
              <a className="font-medium text-accent-ink underline underline-offset-4" href={privacyContact}>
                {contact}
              </a>
            </p>
          </article>
        </div>
      </main>
      <Footer page="privacy" />
    </>
  );
}
