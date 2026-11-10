// 404.html: one page serves every missing address, so it cannot know the reader's language; it says it in every language there is, English first.
import site from "virtual:kando/site";
import { pagePath } from "../../lib/site";
import { Footer } from "../footer/Footer";
import { Header } from "../header/Header";

export function NotFoundPage() {
  const [first, ...rest] = site.missing;
  return (
    <>
      <Header page="404" />
      <main id="main" className="missing">
        <div className="wrap">
          <img src="/logo.png" width="72" height="72" alt="" />
          {first && (
            <>
              <h1>{first.title}</h1>
              <p className="lead">{first.body}</p>
              <p>
                <a className="btn primary" href={pagePath(first.code)}>
                  {first.home}
                </a>
              </p>
            </>
          )}
          {rest.length > 0 && (
            <ul className="missing-langs">
              {rest.map((m) => (
                <li key={m.code} lang={m.tag}>
                  <strong>{m.title}</strong>{" "}
                  <a href={pagePath(m.code)} hrefLang={m.tag} data-lang={m.code}>
                    {m.home}
                  </a>
                </li>
              ))}
            </ul>
          )}
        </div>
      </main>
      <Footer page="404" />
    </>
  );
}
