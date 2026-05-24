import type { PageLoad } from "./$types";

export const load = (({ url }) => {
  return { q: url.searchParams.get("q") ?? "" };
}) satisfies PageLoad;
