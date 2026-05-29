import type { PageServerLoad } from './$types';
import { requireRoles } from '$lib/server/guard';

export const load: PageServerLoad = (event) => {
  requireRoles(event, ['WAREHOUSE_STAFF', 'MANAGER', 'ADMIN']);
};
