<script lang="ts">
  import { page } from '$app/stores';
  import { createQuery } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { createConnectTransport } from '@connectrpc/connect-web';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { QCService } from '$lib/gen/simaops/qc/v1/qc_pb';

  const transport = createConnectTransport({ baseUrl: 'http://localhost:8080', useBinaryFormat: false });
  const lotClient = createClient(LotService, transport);
  const qcClient = createClient(QCService, transport);

  const lotId = $derived($page.params.id);

  // Fetch lot details
  const lotQuery = createQuery({
    queryKey: ['lot', lotId],
    queryFn: () => lotClient.getLot({ lotId })
  });

  // Fetch QC result (we need the job ID — for now we'll use lot_id as a proxy via GetQCResult)
  // The QC job list for this lot gives us the job ID
  const qcResultQuery = createQuery({
    queryKey: ['qc-result', lotId],
    queryFn: async () => {
      // First get the lot's QC jobs via listing lots by status (we know it's in QC_REVIEW)
      // For now, use the lot_id directly — the backend GetQCResult takes qc_job_id
      // We'll need to find the job first. Use a convention: pass lot_id and let backend handle it.
      // Actually, we need to get the job ID. Let's try fetching result with lot_id as job_id
      // This is a temporary workaround — in production we'd have a ListQCJobsByLot RPC
      // For the demo, the /qc/[id] page receives the lot_id, and we need to find the job.
      // Simplest: try GetQCResult with lot_id (won't work since it expects job_id)
      // Better: we'll just show the result data that we can get
      return null;
    },
    enabled: false // disabled until we have a proper way to get job_id from lot_id
  });

  const recommendationLabels: Record<number, { text: string; color: string }> = {
    1: { text: 'PASS', color: 'bg-green-100 text-green-800' },
    2: { text: 'REVIEW', color: 'bg-yellow-100 text-yellow-800' },
    3: { text: 'FAIL', color: 'bg-red-100 text-red-800' }
  };
</script>

<div class="max-w-4xl space-y-6">
  {#if $lotQuery.isLoading}
    <p class="text-gray-500">Loading...</p>
  {:else if $lotQuery.data?.lot}
    {@const lot = $lotQuery.data.lot}

    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">QC Review: {lot.lotNumber}</h1>
        <p class="text-gray-500 text-sm">{lot.materialName} — {lot.supplierName}</p>
      </div>
      <a href="/qc" class="text-sm text-blue-600 hover:underline">← Back to QC queue</a>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <!-- Left: Image -->
      <div class="border rounded-lg p-4">
        <h2 class="font-semibold text-sm text-gray-500 uppercase mb-3">QC Image</h2>
        <div class="bg-gray-100 rounded-lg h-64 flex items-center justify-center text-gray-400">
          <p class="text-sm">Image preview (requires MinIO presigned GET — Task 29)</p>
        </div>
      </div>

      <!-- Right: AI Findings -->
      <div class="border rounded-lg p-4 space-y-4">
        <h2 class="font-semibold text-sm text-gray-500 uppercase">AI Recommendation</h2>

        <!-- Mock data display (since we can't fetch result without job_id yet) -->
        <div class="space-y-3">
          <div class="flex items-center gap-3">
            <span class="text-sm font-medium">Recommendation:</span>
            <span class="px-2 py-1 rounded text-xs font-bold bg-yellow-100 text-yellow-800">REVIEW</span>
          </div>

          <div class="flex items-center gap-3">
            <span class="text-sm font-medium">Confidence:</span>
            <span class="text-sm">82%</span>
          </div>

          <div class="flex items-center gap-3">
            <span class="text-sm font-medium">Model:</span>
            <span class="text-xs font-mono bg-gray-100 px-2 py-0.5 rounded">mock-v0.1.0</span>
          </div>

          <div>
            <span class="text-sm font-medium">Findings:</span>
            <div class="mt-2 space-y-1">
              <div class="flex items-center gap-2">
                <span class="w-2 h-2 rounded-full bg-red-500"></span>
                <span class="text-sm">foreign_matter</span>
                <span class="text-xs text-gray-400">(bottle, 87%)</span>
                <span class="px-1.5 py-0.5 rounded text-xs bg-red-50 text-red-600">anomaly</span>
              </div>
              <div class="flex items-center gap-2">
                <span class="w-2 h-2 rounded-full bg-green-500"></span>
                <span class="text-sm">ripeness_signal</span>
                <span class="text-xs text-gray-400">(banana, 92%)</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Supervisor Actions -->
        <div class="border-t pt-4 space-y-3">
          <h3 class="font-semibold text-sm">Supervisor Decision</h3>
          <div class="flex gap-2">
            <a href="/qc/{lotId}/review?decision=approve"
              class="px-4 py-2 bg-green-600 text-white rounded-md text-sm hover:bg-green-700">
              ✓ Approve
            </a>
            <a href="/qc/{lotId}/review?decision=reject"
              class="px-4 py-2 bg-red-600 text-white rounded-md text-sm hover:bg-red-700">
              ✗ Reject
            </a>
            <a href="/qc/{lotId}/review?decision=recheck"
              class="px-4 py-2 border border-gray-300 rounded-md text-sm hover:bg-gray-50">
              ↻ Recheck
            </a>
          </div>
          <p class="text-xs text-gray-400">Approve/Reject actions implemented in Task 11</p>
        </div>
      </div>
    </div>
  {/if}
</div>
