<script lang="ts">
  import { page } from '$app/stores';
  import { goto } from '$app/navigation';
  import { createQuery, getQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { QCService } from '$lib/gen/simaops/qc/v1/qc_pb';

  const lotClient = createClient(LotService, transport);
  const qcClient = createClient(QCService, transport);
  const queryClient = getQueryClientContext();

  const lotId = $derived($page.params.id);

  const lotQuery = createQuery(() => ({
    queryKey: ['lot', lotId],
    queryFn: () => lotClient.getLot({ lotId })
  }));

  // Fetch the latest QC job for this lot
  const qcJobQuery = createQuery(() => ({
    queryKey: ['qc-job-for-lot', lotId],
    queryFn: () => qcClient.getQCJob({ lotId, qcJobId: '' }),
    enabled: !!lotId
  }));

  const qcResultQuery = createQuery(() => ({
    queryKey: ['qc-result', qcJobQuery.data?.job?.id],
    queryFn: () => qcClient.getQCResult({ qcJobId: qcJobQuery.data!.job!.id }),
    enabled: !!qcJobQuery.data?.job?.id
  }));

  // Review modal state
  let showModal = $state(false);
  let decision = $state(0); // SupervisorDecision enum value
  let reason = $state('');
  let submitting = $state(false);
  let reviewError = $state('');
  let reviewSuccess = $state('');

  const qcJobId = $derived(qcJobQuery.data?.job?.id ?? '');
  const aiResult = $derived(qcResultQuery.data?.result);

  function openReview(d: number) {
    decision = d;
    reason = '';
    reviewError = '';
    showModal = true;
  }

  async function submitReview() {
    if (!qcJobId) {
      reviewError = 'No QC job found for this lot';
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
      reviewSuccess = decision === 1 ? 'Approved' : decision === 2 ? 'Rejected' : 'Recheck requested';
      queryClient.invalidateQueries({ queryKey: ['lot', lotId] });
      queryClient.invalidateQueries({ queryKey: ['qc-job-for-lot', lotId] });
    } catch (e: any) {
      reviewError = e.message || 'Review failed';
    } finally {
      submitting = false;
    }
  }

  const decisionLabels: Record<number, string> = { 1: 'Approve', 2: 'Reject', 3: 'Recheck' };
  const decisionColors: Record<number, string> = {
    1: 'bg-green-600 hover:bg-green-700',
    2: 'bg-red-600 hover:bg-red-700',
    3: 'bg-gray-600 hover:bg-gray-700'
  };
</script>

<div class="max-w-4xl space-y-6">
  {#if lotQuery.isLoading}
    <p class="text-gray-500">Loading...</p>
  {:else if lotQuery.data?.lot}
    {@const lot = lotQuery.data.lot}

    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">QC Review: {lot.lotNumber}</h1>
        <p class="text-gray-500 text-sm">{lot.materialName} — {lot.supplierName}</p>
      </div>
      <a href="/qc" class="text-sm text-blue-600 hover:underline">← Back to QC queue</a>
    </div>

    {#if reviewSuccess}
      <div class="p-3 bg-green-50 border border-green-200 rounded text-green-700 text-sm">
        ✓ Decision recorded: {reviewSuccess}. Lot status updated.
      </div>
    {/if}

    <div class="grid grid-cols-2 gap-6">
      <!-- Left: Image -->
      <div class="border rounded-lg p-4">
        <h2 class="font-semibold text-sm text-gray-500 uppercase mb-3">QC Image</h2>
        <div class="bg-gray-100 rounded-lg h-64 flex items-center justify-center text-gray-400">
          <p class="text-sm text-center px-4">Image preview<br/><span class="text-xs">(presigned GET in Task 29)</span></p>
        </div>
      </div>

      <!-- Right: AI Findings -->
      <div class="border rounded-lg p-4 space-y-4">
        <h2 class="font-semibold text-sm text-gray-500 uppercase">AI Recommendation</h2>

        {#if qcJobQuery.isLoading || qcResultQuery.isLoading}
          <p class="text-sm text-gray-400">Loading AI results...</p>
        {:else if !qcJobId}
          <p class="text-sm text-gray-500">No QC job found for this lot. Upload an image first from the lot detail page.</p>
        {:else if !aiResult}
          <p class="text-sm text-gray-500">AI processing in progress... <span class="text-xs">(refresh in a moment)</span></p>
        {:else}
          {@const recLabels = { 0: 'UNSPECIFIED', 1: 'PASS', 2: 'REVIEW', 3: 'FAIL' }}
          {@const recColors = { 1: 'bg-green-100 text-green-800', 2: 'bg-yellow-100 text-yellow-800', 3: 'bg-red-100 text-red-800' }}
          <div class="space-y-3">
            <div class="flex items-center gap-3">
              <span class="text-sm font-medium">Recommendation:</span>
              <span class="px-2 py-1 rounded text-xs font-bold {recColors[aiResult.recommendation] ?? 'bg-gray-100'}">
                {recLabels[aiResult.recommendation] ?? 'UNKNOWN'}
              </span>
            </div>
            <div class="flex items-center gap-3">
              <span class="text-sm font-medium">Confidence:</span>
              <span class="text-sm">{Math.round((aiResult.confidence ?? 0) * 100)}%</span>
            </div>
            <div class="flex items-center gap-3">
              <span class="text-sm font-medium">Model:</span>
              <span class="text-xs font-mono bg-gray-100 px-2 py-0.5 rounded">{aiResult.modelVersion ?? 'unknown'}</span>
            </div>
            {#if aiResult.findings && aiResult.findings.length > 0}
              <div>
                <span class="text-sm font-medium">Findings:</span>
                <div class="mt-2 space-y-1">
                  {#each aiResult.findings as f}
                    <div class="flex items-center gap-2">
                      <span class="w-2 h-2 rounded-full {f.isAnomaly ? 'bg-red-500' : 'bg-green-500'}"></span>
                      <span class="text-sm">{f.label}</span>
                      <span class="text-xs text-gray-400">({Math.round((f.confidence ?? 0) * 100)}%)</span>
                      {#if f.isAnomaly}
                        <span class="px-1.5 py-0.5 rounded text-xs bg-red-50 text-red-600">anomaly</span>
                      {/if}
                    </div>
                  {/each}
                </div>
              </div>
            {/if}
          </div>
        {/if}

        <!-- Supervisor Actions -->
        {#if !reviewSuccess && qcJobId}
          <div class="border-t pt-4 space-y-3">
            <h3 class="font-semibold text-sm">Supervisor Decision</h3>
            <div class="flex gap-2">
              <button onclick={() => openReview(1)} class="px-4 py-2 bg-green-600 text-white rounded-md text-sm hover:bg-green-700">✓ Approve</button>
              <button onclick={() => openReview(2)} class="px-4 py-2 bg-red-600 text-white rounded-md text-sm hover:bg-red-700">✗ Reject</button>
              <button onclick={() => openReview(3)} class="px-4 py-2 border border-gray-300 rounded-md text-sm hover:bg-gray-50">↻ Recheck</button>
            </div>
          </div>
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- Review Modal -->
{#if showModal}
  <div class="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
    <div class="bg-white rounded-lg p-6 w-full max-w-md space-y-4">
      <h2 class="text-lg font-bold">Confirm: {decisionLabels[decision]}</h2>

      <div>
        <label class="block text-sm font-medium mb-1">
          Reason {decision === 2 ? '(required)' : decision === 1 ? '(required — overriding AI REVIEW)' : '(optional)'}
        </label>
        <textarea
          bind:value={reason}
          rows="3"
          class="w-full border rounded-md px-3 py-2 text-sm"
          placeholder="Explain your decision..."
        ></textarea>
      </div>

      {#if reviewError}
        <p class="text-sm text-red-600">{reviewError}</p>
      {/if}

      <div class="flex gap-3 justify-end">
        <button onclick={() => showModal = false} class="px-4 py-2 border rounded-md text-sm">Cancel</button>
        <button
          onclick={submitReview}
          disabled={submitting}
          class="px-4 py-2 text-white rounded-md text-sm disabled:opacity-50 {decisionColors[decision]}"
        >
          {submitting ? 'Submitting...' : `Confirm ${decisionLabels[decision]}`}
        </button>
      </div>
    </div>
  </div>
{/if}
