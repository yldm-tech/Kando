// The app's mark and name, as the header and the footer show them: the name in the page's language, with Kando after it where that is not Kando.
import { useWords } from "../../i18n/content";

// first marks the header's, the first image of the page and its largest paint on a phone, which the browser then fetches first.
export function Brand({ first = false }: { first?: boolean }) {
  const { t } = useWords();
  const name = t("brandName");
  return (
    <>
      <img src="/logo.png" width="34" height="34" alt="" fetchPriority={first ? "high" : undefined} />
      <strong>{name}</strong>
      {name !== "Kando" && <span>Kando</span>}
    </>
  );
}
