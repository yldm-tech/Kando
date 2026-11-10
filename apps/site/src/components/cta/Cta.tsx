// The closing call to download.
import { useWords } from "../../i18n/content";
import { latestURL } from "../../lib/site";
import { AppleMark } from "../common/marks";

export function Cta() {
  const { t } = useWords();
  return (
    <section className="cta" aria-labelledby="cta-title">
      <div className="cta-sky" aria-hidden="true">
        <span className="blob blob-d" />
        <span className="blob blob-e" />
      </div>
      <div className="wrap cta-in rv">
        <img className="cta-icon" src="/logo.png" width="104" height="104" alt="" />
        <h2 id="cta-title">{t("ctaTitle")}</h2>
        <p className="lead">{t("ctaLead")}</p>
        <p>
          <a className="btn primary huge" href={latestURL}>
            <AppleMark size={18} />
            {t("heroDownload")}
            <span className="shine" aria-hidden="true" />
          </a>
        </p>
        <p className="fine">
          {t("ctaNote") + " "}
          <a href="#start" data-os="linux">
            {t("ctaOther")}
          </a>
        </p>
      </div>
    </section>
  );
}
