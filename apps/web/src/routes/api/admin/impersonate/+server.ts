import { error, json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { COOKIE_OPTS } from '$lib/server/auth';

// Admin-only: start/stop impersonating another user. The cookie is httpOnly and
// only meaningful to the API when the REAL token is ADMIN, so it can't be used
// to escalate by a non-admin even if forged.
function requireAdmin(locals: App.Locals) {
  const roles = locals.user?.roles ?? [];
  if (!locals.accessToken) throw error(401, 'unauthenticated');
  if (!roles.includes('ADMIN')) throw error(403, 'admin only');
}

// POST { username } — begin impersonating that user.
export const POST: RequestHandler = async ({ request, locals, cookies }) => {
  requireAdmin(locals);
  const { username } = await request.json().catch(() => ({ username: '' }));
  if (!username || typeof username !== 'string') throw error(400, 'username required');
  if (username === locals.user?.username) throw error(400, 'cannot impersonate yourself');
  cookies.set('impersonate', username, { ...COOKIE_OPTS, maxAge: 3600 });
  return json({ impersonating: username });
};

// DELETE — stop impersonating.
export const DELETE: RequestHandler = async ({ locals, cookies }) => {
  requireAdmin(locals);
  cookies.delete('impersonate', { ...COOKIE_OPTS });
  return json({ impersonating: null });
};
