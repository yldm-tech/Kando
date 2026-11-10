// A language's home page, the landing page.
import { useRef } from "react";
import { Cli } from "../cli/Cli";
import { Cta } from "../cta/Cta";
import { Details } from "../details/Details";
import { Faq } from "../faq/Faq";
import { Footer } from "../footer/Footer";
import { Header } from "../header/Header";
import { Hero } from "../hero/Hero";
import { useReveals } from "../common/motion";
import { Library } from "../library/Library";
import { OpenSource } from "../open-source/OpenSource";
import { SitesStrip } from "../sites-strip/SitesStrip";
import { Steps } from "../steps/Steps";

export function HomePage() {
  const main = useRef<HTMLElement>(null);
  useReveals(main);
  return (
    <>
      <Header page="home" />
      <main id="main" ref={main}>
        <Hero />
        <SitesStrip />
        <Steps />
        <Library />
        <Cli />
        <Details />
        <OpenSource />
        <Faq />
        <Cta />
      </main>
      <Footer page="home" />
    </>
  );
}
