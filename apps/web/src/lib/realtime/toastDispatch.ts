import type { RealtimeEvent } from '$lib/realtime.svelte';
import { pushToast, type ToastSpec } from '$lib/components/Toaster.svelte';
import { markSeen } from './dedup';

// Role-targeted toast dispatch.
//
// For each event subject, decide:
//   - which roles should see a toast (subset of allowed-by-server roles)
//   - what the toast text/href looks like (i18n-key strings, resolved by caller)
//
// The realtime store calls dispatchToast(event, roles, t) for every incoming
// event. We dedup by event_id across browser tabs via localStorage so a user
// with three tabs open doesn't get three identical toasts.

interface Spec {
  /** roles that should see the toast (subset of server-allowed) */
  roles: string[];
  /** factory that builds a localized toast spec */
  build: (e: RealtimeEvent, t: TranslateFn) => Omit<ToastSpec, 'id'>;
  /** if non-null, only owner-matched operators get the toast (lot creator) */
  operatorOwnerOnly?: boolean;
}

type TranslateFn = (key: string, opts?: { values?: Record<string, any> }) => string;

const dispatch: Record<string, Spec> = {
  'qc.job.needs_human_review': {
    roles: ['QC_SUPERVISOR', 'MANAGER', 'ADMIN'],
    build: (e, t) => ({
      title: t('toast.qc_needs_review.title'),
      body: t('toast.qc_needs_review.body', {
        values: { lot: e.envelope.payload?.lot_number ?? '' },
      }),
      href: `/qc/${e.envelope.resource_id}`,
      variant: 'warning',
    }),
  },
  'qc.job.approved': {
    roles: ['WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'],
    build: (e, t) => ({
      title: t('toast.qc_approved.title'),
      body: t('toast.qc_approved.body', {
        values: { lot: e.envelope.payload?.lot_number ?? '' },
      }),
      href: `/warehouse?lot=${e.envelope.payload?.lot_id ?? ''}`,
      variant: 'success',
    }),
  },
  'qc.job.failed': {
    roles: ['MANAGER', 'ADMIN'], // operator (owner) gets it via operatorOwnerOnly path
    operatorOwnerOnly: true,
    build: (e, t) => ({
      title: t('toast.qc_failed.title'),
      body: t('toast.qc_failed.body', {
        values: { lot: e.envelope.payload?.lot_number ?? '' },
      }),
      href: `/qc/${e.envelope.resource_id}`,
      variant: 'error',
    }),
  },
  'warehouse.slot_assigned': {
    roles: ['MANAGER', 'ADMIN'],
    operatorOwnerOnly: true,
    build: (e, t) => ({
      title: t('toast.warehouse_assigned.title'),
      body: t('toast.warehouse_assigned.body', {
        values: {
          lot: e.envelope.payload?.lot_number ?? '',
          slot: e.envelope.payload?.location_code ?? '',
        },
      }),
      href: `/lots/${e.envelope.payload?.lot_id ?? ''}`,
      variant: 'success',
    }),
  },
  'lot.ready_for_production': {
    roles: ['WAREHOUSE_STAFF', 'MANAGER', 'ADMIN'],
    build: (e, t) => ({
      title: t('toast.ready_for_production.title'),
      body: t('toast.ready_for_production.body', {
        values: { lot: e.envelope.payload?.lot_number ?? '' },
      }),
      href: '/dispatch',
      variant: 'success',
    }),
  },
  'dispatch.status_changed': {
    roles: ['MANAGER', 'ADMIN'],
    operatorOwnerOnly: true,
    build: (e, t) => ({
      title: t('toast.dispatch_status.title'),
      body: t('toast.dispatch_status.body', {
        values: {
          dispatch: e.envelope.payload?.dispatch_number ?? '',
          status: e.envelope.payload?.to ?? '',
        },
      }),
      href: '/dispatch',
      variant: 'info',
    }),
  },
};

export interface DispatchContext {
  userSub: string;
  roles: string[];
  t: TranslateFn;
}

export function dispatchToast(event: RealtimeEvent, ctx: DispatchContext) {
  const spec = dispatch[event.subject];
  if (!spec) return;

  const isOwner = event.envelope.owner_user_id && event.envelope.owner_user_id === ctx.userSub;
  const isOperator = ctx.roles.includes('OPERATOR') && !ctx.roles.includes('ADMIN') && !ctx.roles.includes('MANAGER');

  // Decide if THIS user should see the toast.
  const matchesPrimaryRoles = ctx.roles.some((r) => spec.roles.includes(r));
  const matchesOperatorOwner = !!spec.operatorOwnerOnly && isOperator && isOwner;
  if (!matchesPrimaryRoles && !matchesOperatorOwner) return;

  // Cross-tab dedup.
  if (!markSeen(event.envelope.event_id)) return;

  pushToast(spec.build(event, ctx.t));
}
