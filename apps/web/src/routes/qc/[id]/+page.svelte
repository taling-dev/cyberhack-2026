<script lang="ts">
  import { page } from '$app/stores';
  import { t } from 'svelte-i18n';
  import { get } from 'svelte/store';
  import { createQuery, getQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { QCService } from '$lib/gen/simaops/qc/v1/qc_pb';
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

  // Latest QC job for this lot. Poll only while the job is non-terminal —
  // once it reaches APPROVED(5)/REJECTED(6)/FAILED(7) there's nothing left to
  // wait for, so stop hitting the API every 5s.
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
    // The result is immutable once present; stop polling as soon as we have it.
    refetchInterval: (q) => (q.state.data?.result ? false : 5_000),
  }));

  // Presigned GET for image
  const imageUrlQuery = createQuery(() => ({
    queryKey: ['qc-image-url', qcJobQuery.data?.job?.imageObjectKey],
    queryFn: () => qcClient.createQCViewUrl({ objectKey: qcJobQuery.data!.job!.imageObjectKey }),
    enabled: !!qcJobQuery.data?.job?.imageObjectKey,
    staleTime: 10 * 60 * 1000, // URL valid 15min, refetch after 10
  }));

  let showModal = $state(false);
  let decision = $state(0);
  let reason = $state('');
  let submitting = $state(false);
  let reviewError = $state('');
  let reviewSuccess = $state('');

  const qcJobId = $derived(qcJobQuery.data?.job?.id ?? '');
  const aiResult = $derived(qcResultQuery.data?.result);
  const imageUrl = $derived(imageUrlQuery.data?.viewUrl ?? '');

  function openReview(d: number) {
    decision = d;
    reason = '';
    reviewError = '';
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
      queryClient.invalidateQueries({ queryKey: ['qc-job-for-lot', lotId] });
    } catch (e: any) {
      reviewError = e.message || $t('qc.review_failed');
    } finally {
      submitting = false;
    }
  }

  const decisionColors: Record<number, string> = {
    1: 'bg-green-600 hover:bg-green-700',
    2: 'bg-red-600 hover:bg-red-700',
    3: 'bg-gray-600 hover:bg-gray-700'
  };

  const recColors: Record<number, string> = {
    1: 'bg-green-100 text-green-800',
    2: 'bg-yellow-100 text-yellow-800',
    3: 'bg-red-100 text-red-800',
  };
</script>

<svelte:window onkeydown={handleKeydown} />

<div class="max-w-5xl space-y-6">
  {#if lotQuery.isLoading}
    <p class="text-gray-500">{$t('common.loading')}</p>
  {:else if lotQuery.isError}
    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm">{lotQuery.error?.message || $t('common.error')}</div>
  {:else if lotQuery.data?.lot}
    {@const lot = lotQuery.data.lot}

    <div class="flex items-center justify-between">
      <div>
        <a href="/qc" class="text-sm text-gray-500 hover:text-blue-600">{$t('qc.back_to_queue')}</a>
        <h1 class="text-2xl font-bold mt-1">{$t('qc.review_for')}: {lot.lotNumber}</h1>
        <p class="text-gray-500 text-sm">{lot.materialName} — {lot.supplierName}</p>
      </div>
      <span class="px-3 py-1 rounded-full text-sm font-medium bg-blue-100 text-blue-700">
        {$t(`lot_status.${lot.status}`)}
      </span>
    </div>

    {#if reviewSuccess}
      <div class="p-3 bg-green-50 border border-green-200 rounded text-green-700 text-sm" role="status">
        ✓ {$t('qc.decision_recorded')}: {reviewSuccess}
      </div>
    {/if}

    <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
      <!-- Left: Image -->
      <div class="border rounded-lg p-4 bg-white">
        <h2 class="font-semibold text-sm text-gray-500 uppercase mb-3">{$t('qc.qc_image')}</h2>
        <div class="bg-gray-100 rounded-lg min-h-64 flex items-center justify-center text-gray-400">
          {#if imageUrlQuery.isLoading}
            <p class="text-sm">{$t('qc.image_loading')}</p>
          {:else if imageUrl}
            <img
              src={imageUrl}
              alt="QC image for lot {lot.lotNumber}"
              class="max-w-full max-h-[500px] object-contain rounded-lg"
            />
          {:else}
            <p class="text-sm text-center px-4">{$t('qc.image_unavailable')}</p>
          {/if}
        </div>
      </div>

      <!-- Right: AI Findings -->
      <div class="border rounded-lg p-4 bg-white space-y-4">
        <h2 class="font-semibold text-sm text-gray-500 uppercase">{$t('qc.ai_recommendation')}</h2>

        {#if qcJobQuery.isLoading || qcResultQuery.isLoading}
          <p class="text-sm text-gray-400">{$t('qc.ai_loading')}</p>
        {:else if !qcJobId}
          <p class="text-sm text-gray-500">{$t('qc.ai_no_job')}</p>
        {:else if !aiResult}
          <p class="text-sm text-gray-500">{$t('qc.ai_processing')} <span class="text-xs">{$t('qc.ai_recheck_hint')}</span></p>
        {:else}
          <div class="space-y-3">
            <div class="flex items-center gap-3">
              <span class="text-sm font-medium">{$t('qc.recommendation')}:</span>
              <span class="px-2 py-1 rounded text-xs font-bold {recColors[aiResult.recommendation] ?? 'bg-gray-100'}">
                {aiResult.recommendation === 1 ? 'PASS' : aiResult.recommendation === 2 ? 'REVIEW' : aiResult.recommendation === 3 ? 'FAIL' : '—'}
              </span>
            </div>
            <div class="flex items-center gap-3">
              <span class="text-sm font-medium">{$t('qc.confidence')}:</span>
              <span class="text-sm">{Math.round((aiResult.confidence ?? 0) * 100)}%</span>
            </div>
            <div class="flex items-center gap-3">
              <span class="text-sm font-medium">{$t('qc.model')}:</span>
              <span class="text-xs font-mono bg-gray-100 px-2 py-0.5 rounded">{aiResult.modelVersion ?? 'unknown'}</span>
            </div>
            {#if aiResult.findings && aiResult.findings.length > 0}
              <div>
                <span class="text-sm font-medium">{$t('qc.findings')}:</span>
                <div class="mt-2 space-y-1">
                  {#each aiResult.findings as f}
                    <div class="flex items-center gap-2">
                      <span class="w-2 h-2 rounded-full {f.isAnomaly ? 'bg-red-500' : 'bg-green-500'}" aria-hidden="true"></span>
                      <span class="text-sm">{f.mappedFinding || f.className}</span>
                      <span class="text-xs text-gray-400">({Math.round((f.confidence ?? 0) * 100)}%)</span>
                      {#if f.isAnomaly}
                        <span class="px-1.5 py-0.5 rounded text-xs bg-red-50 text-red-600">{$t('qc.anomaly')}</span>
                      {/if}
                    </div>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
        {/if}

        <!-- Supervisor Actions -->
        {#if !reviewSuccess && qcJobId && lot.status === 4}
          <div class="border-t pt-4 space-y-3">
            <h3 class="font-semibold text-sm">{$t('qc.supervisor_decision')}</h3>
            <div class="flex gap-2 flex-wrap">
              <button onclick={() => openReview(1)} class="px-4 py-2 bg-green-600 text-white rounded-md text-sm hover:bg-green-700">{$t('qc.approve')}</button>
              <button onclick={() => openReview(2)} class="px-4 py-2 bg-red-600 text-white rounded-md text-sm hover:bg-red-700">{$t('qc.reject')}</button>
              <button onclick={() => openReview(3)} class="px-4 py-2 border border-gray-300 rounded-md text-sm hover:bg-gray-50">{$t('qc.recheck')}</button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- Review Modal -->
{#if showModal}
  <div
    class="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="modal-title"
    tabindex="-1"
    use:focusTrap
    onclick={(e) => { if (e.target === e.currentTarget) showModal = false; }}
    onkeydown={(e) => { if (e.key === 'Escape') showModal = false; }}
  >
    <div class="bg-white rounded-lg p-6 w-full max-w-md space-y-4 shadow-xl">
      <h2 id="modal-title" class="text-lg font-bold">
        {$t('qc.confirm_label')}: {decision === 1 ? $t('qc.approve').replace('✓ ','') : decision === 2 ? $t('qc.reject').replace('✗ ','') : $t('qc.recheck').replace('↻ ','')}
      </h2>

      <div>
        <label for="reason-input" class="block text-sm font-medium mb-1">
          {$t('qc.reason')}
          {#if decision === 2}
            <span class="text-red-500">{$t('qc.reason_required')}</span>
          {:else if decision === 1}
            <span class="text-orange-600">{$t('qc.reason_required_override')}</span>
          {:else}
            <span class="text-gray-400">{$t('qc.reason_optional')}</span>
          {/if}
        </label>
        <textarea
          id="reason-input"
          bind:value={reason}
          rows="3"
          class="w-full border rounded-md px-3 py-2 text-sm"
          placeholder={$t('qc.reason_placeholder')}
        ></textarea>
      </div>

      {#if reviewError}
        <p class="text-sm text-red-600" role="alert">{reviewError}</p>
      {/if}

      <div class="flex gap-3 justify-end">
        <button onclick={() => showModal = false} class="px-4 py-2 border rounded-md text-sm">{$t('common.cancel')}</button>
        <button
          onclick={submitReview}
          disabled={submitting || (decision === 2 && !reason.trim())}
          class="px-4 py-2 text-white rounded-md text-sm disabled:opacity-50 {decisionColors[decision]}"
        >
          {submitting ? $t('qc.submitting') : $t('qc.confirm_label')}
        </button>
      </div>
    </div>
  </div>
{/if}
