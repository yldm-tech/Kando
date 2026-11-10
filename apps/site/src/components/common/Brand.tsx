// The app's mark and name, as the header and the footer show them: the name in the page's language, with Kando after it where that is not Kando.
import { useWords } from "../../i18n/content";

export function Brand() {
  const { t } = useWords();
  const name = t("brandName");
  return (
    <>
      <img src="/logo.png" width="34" height="34" alt="" />
      <strong>{name}</strong>
      {name !== "Kando" && <span>Kando</span>}
    </>
  );
}
