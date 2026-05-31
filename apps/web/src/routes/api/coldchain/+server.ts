import { error, json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';
import { env } from '$env/dynamic/private';

const WI_URL = env.PRIVATE_WAREHOUSE_INTELLIGENCE_URL || 'http://simaops-warehouse-intelligence.simaops:8000';

// Session-gated proxy to the warehouse-intelligence cold-chain status.
export const GET: RequestHandler = async ({ locals, request }) => {
	if (!locals.accessToken) throw error(401, 'unauthenticated');
	try {
		const res = await fetch(`${WI_URL}/coldchain/status`, { signal: request.signal });
		if (!res.ok) throw error(502, 'cold-chain service error');
		return json(await res.json());
	} catch {
		throw error(502, 'cold-chain service unreachable');
	}
};
