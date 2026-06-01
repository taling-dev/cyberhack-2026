<script lang="ts">
  import { t } from 'svelte-i18n';
  import DashboardIcon from './DashboardIcon.svelte';

  type LotLike = {
    id: string;
    lotNumber: string;
    materialName: string;
    materialType: number;
    supplierName: string;
    quantity: number;
    unit: string;
    status: number;
    qcRecommendation?: string;
    qcConfidence?: number;
    createdAt?: { seconds?: bigint | number | string };
  };

  let {
    lots = [],
    loading = false,
  } = $props<{
    lots?: LotLike[];
    loading?: boolean;
  }>();

  function qcTone(rec?: string) {
    if (rec === 'PASS') return 'bg-emerald-100 text-emerald-700';
    if (rec === 'REVIEW') return 'bg-orange-100 text-orange-700';
    if (rec === 'FAIL') return 'bg-red-100 text-red-700';
    return 'bg-slate-100 text-slate-500';
  }

  function statusClass(value: number) {
    if (value === 4) return 'bg-orange-100 text-orange-700';
    if (value === 5 || value === 8) return 'bg-emerald-100 text-emerald-700';
    if (value === 6 || value === 9) return 'bg-red-100 text-red-700';
    if (value === 2) return 'bg-yellow-100 text-yellow-700';
    if (value === 7) return 'bg-purple-100 text-purple-700';
    if (value === 3) return 'bg-blue-100 text-blue-700';
    return 'bg-slate-100 text-slate-700';
  }

  function dateLabel(value?: { seconds?: bigint | number | string }) {
    if (!value?.seconds) return '-';
    return new Date(Number(value.seconds) * 1000).toLocaleDateString('en-US');
  }

  function formatQuantity(value: number) {
    return Number.isInteger(value) ? value.toLocaleString('en-US') : value.toLocaleString('en-US', { maximumFractionDigits: 2 });
  }
</script>

<section class="flex h-full min-h-0 flex-col rounded-lg border border-slate-200 bg-white p-4 shadow-sm">
  <div class="flex items-center justify-between">
    <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">{$t('widgets.newest_lots')}</h2>
    <a href="/lots" class="flex items-center gap-2 rounded-md border border-slate-200 px-3 py-1.5 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50">
      {$t('common.see_all')}
      <DashboardIcon name="arrow-right" class="size-4" />
    </a>
  </div>

  <div class="mt-3 min-h-0 flex-1 overflow-hidden">
    {#if loading}
      <div class="space-y-2">
        {#each [1, 2, 3, 4, 5] as _}
          <div class="h-9 animate-pulse rounded bg-slate-100"></div>
        {/each}
      </div>
    {:else if lots.length === 0}
      <div class="flex h-full items-center justify-center rounded-md bg-slate-50 text-sm text-slate-500">
        {$t('lot.no_lots')}
      </div>
    {:else}
      <div class="h-full overflow-hidden">
        <table class="w-full table-fixed text-left text-xs">
          <thead class="border-b border-slate-200 text-[11px] text-slate-500">
            <tr>
              <th class="w-[17%] pb-2 font-semibold">{$t('lot.lot_number')}</th>
              <th class="w-[16%] pb-2 font-semibold">{$t('widgets.material')}</th>
              <th class="w-[14%] pb-2 font-semibold">{$t('widgets.type')}</th>
              <th class="w-[16%] pb-2 font-semibold">{$t('widgets.supplier')}</th>
              <th class="w-[11%] pb-2 font-semibold">{$t('widgets.qty')}</th>
              <th class="w-[12%] pb-2 font-semibold">{$t('widgets.ai_score')}</th>
              <th class="w-[14%] pb-2 font-semibold">{$t('widgets.status')}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-slate-100">
            {#each lots.slice(0, 5) as lot}
              <tr class="transition-colors hover:bg-slate-50">
                <td class="py-2 pr-2">
                  <a href="/lots/{lot.id}" class="block max-h-8 overflow-hidden break-words font-mono text-[11px] font-medium leading-4 text-blue-600 hover:underline">{lot.lotNumber}</a>
                </td>
                <td class="truncate py-2 pr-2 text-slate-900">{lot.materialName}</td>
                <td class="py-2 pr-2 text-slate-700">{$t(`material_type.${lot.materialType}`)}</td>
                <td class="truncate py-2 pr-2 text-slate-700">{lot.supplierName}</td>
                <td class="whitespace-nowrap py-2 pr-2 text-slate-700">{formatQuantity(lot.quantity)} {lot.unit}</td>
                <td class="py-2 pr-2">
                  {#if lot.qcRecommendation}
                    <span class="rounded-md px-2 py-0.5 text-[11px] font-semibold {qcTone(lot.qcRecommendation)}">{Math.round((lot.qcConfidence ?? 0) * 100)}%</span>
                  {:else}
                    <span class="text-[11px] text-slate-400">—</span>
                  {/if}
                </td>
                <td class="py-2">
                  <span class="inline-block max-w-full truncate rounded-md px-2 py-0.5 text-[11px] font-medium {statusClass(lot.status)}">{$t(`lot_status.${lot.status}`)}</span>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>
</section>
