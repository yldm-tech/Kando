// A language's home page, the landing page.
import { Cli } from "../cli/Cli";
import { Cta } from "../cta/Cta";
import { Details } from "../details/Details";
import { Faq } from "../faq/Faq";
import { Footer } from "../footer/Footer";
import { Header } from "../header/Header";
import { Hero } from "../hero/Hero";
import { Library } from "../library/Library";
import { Models } from "../models/Models";
import { OpenSource } from "../open-source/OpenSource";
import { SitesStrip } from "../sites-strip/SitesStrip";
import { Steps } from "../steps/Steps";

export function HomePage() {
  return (
    <>
      <Header page="home" />
      <main id="main">
        <Hero />
        <SitesStrip />
        <Steps />
        <Models />
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
