import type { PageServerLoad } from "./$types";
import { endpointsFor } from "$lib/api";
import { redirect } from "@sveltejs/kit";

export const load: PageServerLoad = async (event) => {
  const { user } = await event.parent();
  if (!user) throw redirect(303, "/auth/login");
  const category = event.url.searchParams.get("category") ?? "";
  return await endpointsFor(event.fetch).feeds(category);
};
