import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = async ({ locals }) => {
  // When impersonating, locals.user is the impersonated identity and
  // locals.realUser is the admin. Surface the impersonated username for the
  // banner; null when not impersonating.
  return {
    user: locals.user ?? null,
    impersonating: locals.realUser ? (locals.user?.username ?? null) : null
  };
};
