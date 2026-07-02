import type { LayoutServerLoad } from "./$types";
import { endpointsFor } from "$lib/api";

export const load: LayoutServerLoad = async (event) => {
  const api = endpointsFor(event.fetch);
  try {
    const me = await api.me();
    return {
      user: me.user,
      csrfToken: me.csrf_token,
      hasLLM: me.has_llm,
      clientID: me.client_id,
    };
  } catch {
    return { user: null, csrfToken: "", hasLLM: false, clientID: "" };
  }
};
