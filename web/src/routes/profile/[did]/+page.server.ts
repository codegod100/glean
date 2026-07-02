import type { PageServerLoad } from "./$types";
import { endpointsFor } from "$lib/api";
import { redirect, error } from "@sveltejs/kit";

export const load: PageServerLoad = async (event) => {
  const { user } = await event.parent();
  if (!user) throw redirect(303, "/auth/login");
  try {
    return await endpointsFor(event.fetch).profile(event.params.did);
  } catch (e: any) {
    if (e?.status === 404) throw error(404, e.message);
    throw e;
  }
};
