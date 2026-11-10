// The modules the build makes from the repository (buildtime/plugin.ts).

declare module "virtual:kando/site" {
  const site: import("../buildtime/schema").SiteData;
  export default site;
}

declare module "virtual:kando/catalog" {
  const catalog: import("../buildtime/schema").CatalogRow[];
  export default catalog;
}

declare module "virtual:kando/content/*" {
  const content: import("../buildtime/schema").ContentModule;
  export default content;
}

declare module "virtual:kando/content-loaders" {
  const loaders: Partial<Record<import("./lib/site").LangCode, () => Promise<{ default: import("../buildtime/schema").ContentModule }>>>;
  export default loaders;
}
