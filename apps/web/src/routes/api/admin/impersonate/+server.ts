import { error, json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { COOKIE_OPTS } from '$lib/server/auth';

// Admin-only: start/stop impersonating another user. The cookie is httpOnly and
// only meaningful to the API when the REAL token is ADMIN, so it can't be used
// to escalate by a non-admin even if forged.
function requireAdmin(locals: App.Locals) {
  if (!locals.accessToken) throw error(401, 'unauthenticated');
  // While impersonating, the real admin is in realUser; otherwise it's user.
  const effective = locals.realUser ?? locals.user;
  if (!(effective?.roles ?? []).includes('ADMIN')) throw error(403, 'admin only');
}

// POST { username, name, sub, roles } — begin impersonating that user.
export const POST: RequestHandler = async ({ request, locals, cookies }) => {
  requireAdmin(locals);
  const body = await request.json().catch(() => ({}));
  const username = body?.username;
  if (!username || typeof username !== 'string') throw error(400, 'username required');
  const realUsername = (locals.realUser ?? locals.user)?.username;
  if (username === realUsername) throw error(400, 'cannot impersonate yourself');
  // Identity payload mirrors the frontend user shape so locals.user can be
  // overridden in hooks while impersonating. The API independently re-derives
  // the effective user from the DB, so this is display-only and safe.
  const payload = JSON.stringify({
    username,
    name: typeof body?.name === 'string' && body.name ? body.name : username,
    sub: typeof body?.sub === 'string' ? body.sub : '',
    email: typeof body?.email === 'string' ? body.email : '',
    roles: Array.isArray(body?.roles) ? body.roles.filter((r: unknown) => typeof r === 'string') : [],
  });
  cookies.set('impersonate', payload, { ...COOKIE_OPTS, maxAge: 3600 });
  return json({ impersonating: username });
};

// DELETE — stop impersonating.
export const DELETE: RequestHandler = async ({ locals, cookies }) => {
  requireAdmin(locals);
  cookies.delete('impersonate', { ...COOKIE_OPTS });
  return json({ impersonating: null });
};
