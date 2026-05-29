import { error, redirect } from '@sveltejs/kit';
import type { ServerLoadEvent } from '@sveltejs/kit';

/**
 * requireRoles — server-side route guard. Redirects anonymous users to login
 * (preserving return_to) and 403s authenticated users who lack any of the
 * allowed roles. Mirrors the client-side nav gating in +layout.svelte so a
 * direct URL navigation can't render a page shell the user shouldn't see.
 */
export function requireRoles(event: ServerLoadEvent, allowed: string[]): void {
  const user = event.locals.user;
  if (!user) {
    const returnTo = encodeURIComponent(event.url.pathname + event.url.search);
    throw redirect(302, `/auth/login?return_to=${returnTo}`);
  }
  if (!user.roles.some((r) => allowed.includes(r))) {
    throw error(403, 'forbidden');
  }
}
