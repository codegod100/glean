import type { PageServerLoad } from "./$types";
import { endpointsFor } from "$lib/api";
import { redirect, error } from "@sveltejs/kit";

export const load: PageServerLoad = async (event) => {
  const { user } = await event.parent();
  if (!user) throw redirect(303, "/auth/login");

  const id = Number(event.params.id);
  if (!id) throw error(404, "Article not found");

  const query: Record<string, string> = {};
  for (const key of ["from_feed", "liked", "status"]) {
    const v = event.url.searchParams.get(key);
    if (v) query[key] = v;
  }

  try {
    return await endpointsFor(event.fetch).article(id, query);
  } catch (e: any) {
    if (e?.status === 404) throw error(404, "Article not found");
    throw e;
  }
};
