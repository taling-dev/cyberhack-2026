import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

// Lightweight liveness probe — returns 200 immediately without any SSR.
export const GET: RequestHandler = () => {
  return json({ status: 'ok' }, { status: 200 });
};
