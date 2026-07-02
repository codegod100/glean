import type { PageServerLoad } from "./$types";
import { endpointsFor } from "$lib/api";

export const load: PageServerLoad = async (event) => {
  return await endpointsFor(event.fetch).stats();
};
