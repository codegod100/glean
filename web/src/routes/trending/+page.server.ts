import type { PageServerLoad } from "./$types";
import { endpointsFor } from "$lib/api";
import { redirect } from "@sveltejs/kit";

export const load: PageServerLoad = async (event) => {
  const { user } = await event.parent();
  const scope = event.url.searchParams.get("scope") ?? "all";
  if (scope === "for-me" && !user) throw redirect(303, "/auth/login");
  const page = event.url.searchParams.get("page");
  const params: Record<string, string> = { scope };
  if (page) params.page = page;
  return await endpointsFor(event.fetch).trending(params);
};
