<script lang="ts">
  import { page } from '$app/stores';
  import { t } from 'svelte-i18n';
  import { get } from 'svelte/store';
  import { createQuery, getQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { QCService } from '$lib/gen/simaops/qc/v1/qc_pb';
  import DashboardIcon from '$lib/components/dashboard/DashboardIcon.svelte';
  import { focusTrap } from '$lib/actions/focusTrap.svelte';

  const lotClient = createClient(LotService, transport);
  const qcClient = createClient(QCService, transport);
  const queryClient = getQueryClientContext();

  const lotId = $derived($page.params.id);

  const lotQuery = createQuery(() => ({
    queryKey: ['lot', lotId],
    queryFn: () => lotClient.getLot({ lotId }),
    enabled: !!lotId,
  }));

  // Latest QC job for this lot. Poll only while the job is non-terminal:
  // APPROVED(5), REJECTED(6), FAILED(7).
  const TERMINAL_QC_STATUSES = new Set([5, 6, 7]);
  const qcJobQuery = createQuery(() => ({
    queryKey: ['qc-job-for-lot', lotId],
    queryFn: () => qcClient.getQCJob({ lotId, qcJobId: '' }),
    enabled: !!lotId,
    refetchInterval: (q) =>
      TERMINAL_QC_STATUSES.has(q.state.data?.job?.status ?? -1) ? false : 5_000,
  }));

  const qcResultQuery = createQuery(() => ({
    queryKey: ['qc-result', qcJobQuery.data?.job?.id],
    queryFn: () => qcClient.getQCResult({ qcJobId: qcJobQuery.data!.job!.id }),
    enabled: !!qcJobQuery.data?.job?.id,
    refetchInterval: (q) => (q.state.data?.result ? false : 5_000),
  }));

  const imageUrlQuery = createQuery(() => ({
    queryKey: ['qc-image-url', qcJobQuery.data?.job?.imageObjectKey],
    queryFn: () => qcClient.createQCViewUrl({ objectKey: qcJobQuery.data!.job!.imageObjectKey }),
    enabled: !!qcJobQuery.data?.job?.imageObjectKey,
    staleTime: 10 * 60 * 1000,
  }));

  let showModal = $state(false);
  let decision = $state(0);
  let reason = $state('');
  let recheckRerunAi = $state(true);
  let submitting = $state(false);
  let reviewError = $state('');
  let reviewSuccess = $state('');

  const qcJobId = $derived(qcJobQuery.data?.job?.id ?? '');
  const aiResult = $derived(qcResultQuery.data?.result);
  const imageUrl = $derived(imageUrlQuery.data?.viewUrl ?? '');
  const confidencePct = $derived(Math.round((aiResult?.confidence ?? 0) * 100));

  const reasonRequired = $derived(
    decision === 2 ||
    (decision === 1 && (aiResult?.recommendation === 2 || aiResult?.recommendation === 3))
  );

  function openReview(d: number) {
    decision = d;
    reason = '';
    reviewError = '';
    recheckRerunAi = true;
    showModal = true;
  }

  function handleKeydown(e: KeyboardEvent) {
    if (showModal && e.key === 'Escape') {
      showModal = false;
    }
  }

  async function submitReview() {
    if (!qcJobId) {
      reviewError = $t('qc.ai_no_job');
      return;
    }
    submitting = true;
    reviewError = '';
    try {
      await qcClient.reviewQC({
        qcJobId,
        decision,
        reason,
        recheckRerunAi,
        idempotencyKey: crypto.randomUUID()
      });
      showModal = false;
      const tt = get(t);
      reviewSuccess = decision === 1
        ? tt('qc.review_decision_approved')
        : decision === 2
          ? tt('qc.review_decision_rejected')
          : tt('qc.review_decision_recheck');
      queryClient.invalidateQueries({ queryKey: ['lot', lotId] });
      queryClient.invalidateQueries({ queryKey: ['lot-timeline', lotId] });
      queryClient.invalidateQueries({ queryKey: ['qc-job-for-lot', lotId] });
    } catch (e: any) {
      reviewError = $t('qc.request_error');
    } finally {
      submitting = false;
    }
  }

  function recommendationLabel(value?: number) {
    if (value === 1) return 'PASS';
    if (value === 2) return 'REVIEW';
    if (value === 3) return 'FAIL';
    return 'Pending';
  }

  function recommendationClass(value?: number) {
    if (value === 1) return 'bg-emerald-100 text-emerald-700 ring-emerald-200';
    if (value === 2) return 'bg-orange-100 text-orange-700 ring-orange-200';
    if (value === 3) return 'bg-red-100 text-red-700 ring-red-200';
    return 'bg-slate-100 text-slate-600 ring-slate-200';
  }

  function jobStatusLabel(value?: number) {
    return value && value >= 1 && value <= 7 ? $t(`qc.job_status.${value}`) : $t('common.unknown');
  }

  function decisionLabel(value: number) {
    if (value === 1) return 'Approve';
    if (value === 2) return 'Reject';
    if (value === 3) return 'Recheck';
    return $t('qc.confirm_label');
  }

  function decisionTitle(value: number) {
    if (value === 1) return 'Approve inspection';
    if (value === 2) return 'Reject inspection';
    if (value === 3) return 'Request recheck';
    return $t('qc.confirm_label');
  }

  function formatQuantity(value: number) {
    return Number.isInteger(value)
      ? value.toLocaleString('en-US')
      : value.toLocaleString('en-US', { maximumFractionDigits: 2 });
  }

  const decisionButtonClass: Record<number, string> = {
    1: 'bg-emerald-600 hover:bg-emerald-700 focus:ring-emerald-100',
    2: 'bg-red-600 hover:bg-red-700 focus:ring-red-100',
    3: 'bg-slate-700 hover:bg-slate-800 focus:ring-slate-100'
  };
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="space-y-5">
  {#if lotQuery.isLoading}
    <div class="space-y-4">
      <div class="h-20 animate-pulse rounded-lg bg-slate-100"></div>
      <div class="grid gap-4 lg:grid-cols-[1fr_1.05fr]">
        <div class="h-[420px] animate-pulse rounded-lg bg-slate-100"></div>
        <div class="h-[420px] animate-pulse rounded-lg bg-slate-100"></div>
      </div>
    </div>
  {:else if lotQuery.isError}
    <div class="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
      {lotQuery.error?.message || $t('common.error')}
    </div>
  {:else if lotQuery.data?.lot}
    {@const lot = lotQuery.data.lot}

    <header class="flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <a href="/qc" class="inline-flex items-center gap-2 text-sm font-medium text-slate-500 transition-colors hover:text-blue-600">
          <svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M19 12H5" />
            <path d="m12 19-7-7 7-7" />
          </svg>
          Back to QC queue
        </a>
        <p class="mt-4 text-xs font-semibold uppercase tracking-normal text-blue-600">AI Inspection Review</p>
        <h1 class="mt-1 text-[28px] font-bold tracking-normal text-slate-950">{lot.lotNumber}</h1>
        <p class="mt-1 text-sm text-slate-600">{lot.materialName} - {lot.supplierName}</p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <span class="rounded-md border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-500 shadow-sm">
          {formatQuantity(lot.quantity)} {lot.unit}
        </span>
        <span class="inline-flex items-center gap-1.5 rounded-md bg-orange-100 px-2.5 py-1 text-xs font-semibold text-orange-700 ring-1 ring-inset ring-orange-200">
          <span class="size-1.5 rounded-full bg-orange-500"></span>
          {$t(`lot_status.${lot.status}`)}
        </span>
      </div>
    </header>

    {#if reviewSuccess}
      <div class="rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm font-medium text-emerald-700" role="status">
        {$t('qc.decision_recorded')}: {reviewSuccess}
      </div>
    {/if}

    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(420px,.95fr)]">
      <section class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <div class="flex items-center justify-between border-b border-slate-200 px-4 py-3">
          <div>
            <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">{$t('qc.qc_image')}</h2>
            <p class="mt-1 text-xs text-slate-500">Material evidence used by the AI inspection.</p>
          </div>
          <DashboardIcon name="bot" class="size-5 text-slate-400" />
        </div>

        <div class="p-4">
          <div class="flex min-h-[420px] items-center justify-center overflow-hidden rounded-lg bg-slate-100">
            {#if imageUrlQuery.isLoading}
              <div class="flex flex-col items-center text-sm text-slate-500">
                <div class="mb-3 size-8 animate-pulse rounded-md bg-slate-200"></div>
                {$t('qc.image_loading')}
              </div>
            {:else if imageUrl}
              <img
                src={imageUrl}
                alt="QC image for lot {lot.lotNumber}"
                class="max-h-[560px] max-w-full rounded-lg object-contain"
              />
            {:else}
              <div class="flex max-w-sm flex-col items-center px-6 text-center">
                <div class="flex size-14 items-center justify-center rounded-lg bg-slate-200 text-slate-500">
                  <DashboardIcon name="bot" class="size-7" />
                </div>
                <p class="mt-3 text-sm font-semibold text-slate-700">{$t('qc.image_unavailable')}</p>
                <p class="mt-1 text-xs leading-5 text-slate-500">Upload and QC job creation remain available from the lot detail page.</p>
              </div>
            {/if}
          </div>
        </div>
      </section>

      <section class="overflow-hidden rounded-lg border border-slate-200 bg-white shadow-sm">
        <div class="flex items-center justify-between border-b border-slate-200 px-4 py-3">
          <div>
            <h2 class="text-[13px] font-bold uppercase tracking-normal text-slate-950">AI Decision Panel</h2>
            <p class="mt-1 text-xs text-slate-500">Review model output before making a human decision.</p>
          </div>
          <span class="rounded-md bg-purple-100 px-2.5 py-1 text-xs font-semibold text-purple-700">
            {jobStatusLabel(qcJobQuery.data?.job?.status)}
          </span>
        </div>

        <div class="space-y-4 p-4">
          {#if qcJobQuery.isLoading || qcResultQuery.isLoading}
            <div class="space-y-3">
              <div class="h-20 animate-pulse rounded-lg bg-slate-100"></div>
              <div class="h-36 animate-pulse rounded-lg bg-slate-100"></div>
              <div class="h-16 animate-pulse rounded-lg bg-slate-100"></div>
            </div>
          {:else if !qcJobId}
            <div class="flex min-h-64 flex-col items-center justify-center rounded-lg bg-slate-50 px-6 text-center">
              <DashboardIcon name="bot" class="size-8 text-slate-400" />
              <p class="mt-3 text-sm font-semibold text-slate-700">{$t('qc.ai_no_job')}</p>
            </div>
          {:else if !aiResult}
            <div class="flex min-h-64 flex-col items-center justify-center rounded-lg bg-slate-50 px-6 text-center">
              <div class="flex size-12 items-center justify-center rounded-lg bg-blue-100 text-blue-700">
                <DashboardIcon name="activity" class="size-6" />
              </div>
              <p class="mt-3 text-sm font-semibold text-slate-700">{$t('qc.ai_processing')}</p>
              <p class="mt-1 text-xs text-slate-500">{$t('qc.ai_recheck_hint')}</p>
            </div>
          {:else}
            <div class="grid gap-3 sm:grid-cols-2">
              <div class="rounded-lg border border-slate-200 bg-slate-50/70 p-4">
                <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">{$t('qc.recommendation')}</p>
                <span class="mt-3 inline-flex rounded-md px-2.5 py-1 text-xs font-bold ring-1 ring-inset {recommendationClass(aiResult.recommendation)}">
                  {recommendationLabel(aiResult.recommendation)}
                </span>
                <details class="mt-3 text-xs text-slate-500">
                  <summary class="cursor-pointer font-medium text-slate-600 hover:text-slate-950">{$t('qc.rec_help_label')}</summary>
                  <ul class="mt-2 space-y-1">
                    <li class={aiResult.recommendation === 1 ? 'font-semibold text-slate-950' : ''}>{$t('qc.rec_help_pass')}</li>
                    <li class={aiResult.recommendation === 2 ? 'font-semibold text-slate-950' : ''}>{$t('qc.rec_help_review')}</li>
                    <li class={aiResult.recommendation === 3 ? 'font-semibold text-slate-950' : ''}>{$t('qc.rec_help_fail')}</li>
                  </ul>
                </details>
              </div>

              <div class="rounded-lg border border-slate-200 bg-slate-50/70 p-4">
                <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">{$t('qc.confidence')}</p>
                <div class="mt-3 flex items-center gap-3">
                  <span class="text-2xl font-bold text-slate-950">{confidencePct}%</span>
                  <div class="h-2 flex-1 overflow-hidden rounded-full bg-white">
                    <div class="h-full rounded-full bg-blue-600 transition-all" style="width: {confidencePct}%"></div>
                  </div>
                </div>
              </div>
            </div>

            <div class="rounded-lg border border-slate-200 bg-white p-4">
              <div class="flex items-center justify-between gap-3">
                <p class="text-xs font-semibold uppercase tracking-normal text-slate-500">{$t('qc.model')}</p>
                <span class="rounded-md bg-slate-100 px-2 py-1 font-mono text-xs text-slate-600">{aiResult.modelVersion || 'unknown'}</span>
              </div>

              <div class="mt-4">
                <p class="text-sm font-semibold text-slate-950">{$t('qc.findings')}</p>
                {#if aiResult.findings && aiResult.findings.length > 0}
                  <div class="mt-3 space-y-2">
                    {#each aiResult.findings as finding}
                      <div class="flex items-center justify-between gap-3 rounded-md bg-slate-50 px-3 py-2">
                        <div class="min-w-0">
                          <p class="truncate text-sm font-medium text-slate-800">{finding.mappedFinding || finding.className}</p>
                          <p class="mt-0.5 text-xs text-slate-500">{Math.round((finding.confidence ?? 0) * 100)}% confidence</p>
                        </div>
                        <span class="inline-flex shrink-0 items-center gap-1.5 rounded-md px-2 py-1 text-xs font-semibold {finding.isAnomaly ? 'bg-red-100 text-red-700' : 'bg-emerald-100 text-emerald-700'}">
                          <span class="size-1.5 rounded-full {finding.isAnomaly ? 'bg-red-500' : 'bg-emerald-500'}"></span>
                          {finding.isAnomaly ? $t('qc.anomaly') : 'Normal'}
                        </span>
                      </div>
                    {/each}
                  </div>
                {:else}
                  <div class="mt-3 rounded-md bg-slate-50 px-3 py-3 text-sm text-slate-500">
                    No finding checklist returned yet.
                  </div>
                {/if}
              </div>
            </div>
          {/if}

          {#if !reviewSuccess && qcJobId && lot.status === 4}
            <div class="rounded-lg border border-slate-200 bg-slate-50 p-4">
              <h3 class="text-sm font-bold text-slate-950">{$t('qc.supervisor_decision')}</h3>
              <p class="mt-1 text-xs text-slate-500">Record a human decision for this AI inspection result.</p>
              <div class="mt-4 flex flex-wrap gap-2">
                <button onclick={() => openReview(1)} class="inline-flex h-9 items-center gap-2 rounded-md bg-emerald-600 px-3 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-emerald-100">
                  <DashboardIcon name="check-circle" class="size-4" />
                  Approve
                </button>
                <button onclick={() => openReview(2)} class="inline-flex h-9 items-center gap-2 rounded-md bg-red-600 px-3 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-100">
                  Reject
                </button>
                <button onclick={() => openReview(3)} class="inline-flex h-9 items-center gap-2 rounded-md border border-slate-200 bg-white px-3 text-sm font-semibold text-slate-700 shadow-sm transition-colors hover:bg-slate-50">
                  <DashboardIcon name="activity" class="size-4" />
                  Recheck
                </button>
              </div>
            </div>
          {/if}
        </div>
      </section>
    </div>
  {/if}
</div>

{#if showModal}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="modal-title"
    tabindex="-1"
    use:focusTrap
    onclick={(e) => { if (e.target === e.currentTarget) showModal = false; }}
    onkeydown={(e) => { if (e.key === 'Escape') showModal = false; }}
  >
    <div class="max-h-[90vh] w-full max-w-lg overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl">
      <div class="border-b border-slate-200 px-5 py-4">
        <p class="text-xs font-semibold uppercase tracking-normal text-blue-600">Supervisor Decision</p>
        <h2 id="modal-title" class="mt-1 text-lg font-bold text-slate-950">{decisionTitle(decision)}</h2>
      </div>

      <div class="max-h-[62vh] space-y-4 overflow-y-auto px-5 py-4">
        <div>
          <label for="reason-input" class="mb-1 block text-sm font-semibold text-slate-700">
            {$t('qc.reason')}
            {#if decision === 2}
              <span class="text-red-600">{$t('qc.reason_required')}</span>
            {:else if reasonRequired}
              <span class="text-orange-600">{$t('qc.reason_required_override')}</span>
            {:else}
              <span class="text-slate-400">{$t('qc.reason_optional')}</span>
            {/if}
          </label>
          <textarea
            id="reason-input"
            bind:value={reason}
            rows="4"
            class="w-full rounded-md border border-slate-200 bg-white px-3 py-2 text-sm text-slate-700 shadow-sm outline-none transition-colors placeholder:text-slate-400 focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
            placeholder={$t('qc.reason_placeholder')}
          ></textarea>
        </div>

        {#if decision === 3}
          <label class="flex items-start gap-3 rounded-lg border border-slate-200 bg-slate-50 p-3 text-sm">
            <input type="checkbox" bind:checked={recheckRerunAi} class="mt-0.5 rounded border-slate-300 text-blue-600 focus:ring-blue-500" />
            <span>
              <span class="font-medium text-slate-800">{$t('qc.recheck_rerun_ai')}</span>
              <span class="mt-1 block text-xs leading-5 text-slate-500">
                {recheckRerunAi ? $t('qc.recheck_rerun_on_hint') : $t('qc.recheck_rerun_off_hint')}
              </span>
            </span>
          </label>
        {/if}

        {#if reviewError}
          <p class="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700" role="alert">{reviewError}</p>
        {/if}
      </div>

      <div class="flex justify-end gap-2 border-t border-slate-200 bg-slate-50 px-5 py-3">
        <button onclick={() => showModal = false} class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50">
          {$t('common.cancel')}
        </button>
        <button
          onclick={submitReview}
          disabled={submitting || (reasonRequired && !reason.trim())}
          class="h-9 rounded-md px-4 text-sm font-semibold text-white shadow-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50 {decisionButtonClass[decision]}"
        >
          {submitting ? $t('qc.submitting') : decisionLabel(decision)}
        </button>
      </div>
    </div>
  </div>
{/if}
