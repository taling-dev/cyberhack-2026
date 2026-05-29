import type { PageServerLoad } from './$types';
import { requireRoles } from '$lib/server/guard';

export const load: PageServerLoad = (event) => {
  requireRoles(event, ['QC_SUPERVISOR', 'MANAGER', 'ADMIN']);
};
