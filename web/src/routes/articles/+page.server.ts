import type { PageServerLoad } from "./$types";
import { endpointsFor } from "$lib/api";
import { redirect } from "@sveltejs/kit";

export const load: PageServerLoad = async (event) => {
  const { user } = await event.parent();
  if (!user) throw redirect(303, "/auth/login");

  const params: Record<string, string> = {};
  for (const key of ["feed", "status", "q", "sort", "category", "page"]) {
    const v = event.url.searchParams.get(key);
    if (v) params[key] = v;
  }
  return await endpointsFor(event.fetch).articles(params);
};
