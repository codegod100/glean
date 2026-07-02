import type { PageServerLoad } from "./$types";
import { endpointsFor } from "$lib/api";
import { redirect } from "@sveltejs/kit";

export const load: PageServerLoad = async (event) => {
  const { user } = await event.parent();
  if (!user) throw redirect(303, "/auth/login");
  const params: Record<string, string> = {};
  const lp = event.url.searchParams.get("liked_page");
  const ap = event.url.searchParams.get("annot_page");
  if (lp) params.liked_page = lp;
  if (ap) params.annot_page = ap;
  return await endpointsFor(event.fetch).library(params);
};
