<script lang="ts">
  import { page } from '$app/stores';
  import { createQuery, createMutation, useQueryClient } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { createConnectTransport } from '@connectrpc/connect-web';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_connect';
  import { QCService } from '$lib/gen/simaops/qc/v1/qc_connect';
  import { AuditService } from '$lib/gen/simaops/audit/v1/audit_connect';

  const transport = createConnectTransport({ baseUrl: 'http://localhost:8080', useBinaryFormat: false });
  const lotClient = createClient(LotService, transport);
  const qcClient = createClient(QCService, transport);
  const auditClient = createClient(AuditService, transport);
  const queryClient = useQueryClient();

  const lotId = $derived($page.params.id);

  const lotQuery = createQuery({
    queryKey: ['lot', lotId],
    queryFn: () => lotClient.getLot({ lotId })
  });

  const timelineQuery = createQuery({
    queryKey: ['lot-timeline', lotId],
    queryFn: () => auditClient.getEntityAuditTrail({ entityType: 'lot', entityId: lotId })
  });

  // Upload state
  let uploadProgress = $state(0);
  let uploading = $state(false);
  let uploadedKey = $state('');
  let uploadError = $state('');

  async function handleUpload(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;

    uploading = true;
    uploadProgress = 0;
    uploadError = '';

    try {
      // 1. Get presigned URL from API
      const res = await qcClient.createQCUploadUrl({
        lotId,
        filename: file.name,
        contentType: file.type || 'image/jpeg',
        idempotencyKey: crypto.randomUUID()
      });

      // 2. PUT directly to MinIO via presigned URL
      const xhr = new XMLHttpRequest();
      xhr.open('PUT', res.uploadUrl, true);
      xhr.setRequestHeader('Content-Type', file.type || 'image/jpeg');

      xhr.upload.onprogress = (ev) => {
        if (ev.lengthComputable) {
          uploadProgress = Math.round((ev.loaded / ev.total) * 100);
        }
      };

      await new Promise<void>((resolve, reject) => {
        xhr.onload = () => {
          if (xhr.status >= 200 && xhr.status < 300) {
            uploadedKey = res.objectKey;
            resolve();
          } else {
            reject(new Error(`Upload failed: ${xhr.status}`));
          }
        };
        xhr.onerror = () => reject(new Error('Network error'));
        xhr.send(file);
      });
    } catch (err: any) {
      uploadError = err.message || 'Upload failed';
    } finally {
      uploading = false;
    }
  }

  // QC Job state
  let startingQC = $state(false);
  let qcStarted = $state(false);
  let qcError = $state('');

  async function handleStartQC() {
    if (!uploadedKey) return;
    startingQC = true;
    qcError = '';
    try {
      await qcClient.createQCJob({
        lotId,
        imageObjectKey: uploadedKey,
        idempotencyKey: crypto.randomUUID()
      });
      qcStarted = true;
      // Refetch lot to see updated status
      queryClient.invalidateQueries({ queryKey: ['lot', lotId] });
    } catch (e: any) {
      qcError = e.message || 'Failed to start QC';
    } finally {
      startingQC = false;
    }
  }

  const statusLabels: Record<number, string> = {
    1: 'Draft', 2: 'Pending QC', 3: 'AI Processing', 4: 'QC Review',
    5: 'QC Approved', 6: 'QC Rejected', 7: 'Warehouse Assigned', 8: 'Ready for Production', 9: 'Blocked'
  };
  const materialLabels: Record<number, string> = { 1: 'Raw Botanical', 2: 'Extract', 3: 'Powder', 4: 'Other' };
  const tempLabels: Record<number, string> = { 1: 'Ambient (15–25 °C)', 2: 'Cold (2–8 °C)', 3: 'Deep Freeze (−20 to −4 °C)' };
  const hazardLabels: Record<number, string> = { 1: 'None', 2: 'IBC', 3: 'IPPC' };

  // Upload allowed only in certain statuses
  const uploadAllowed = $derived(
    [1, 2, 6].includes($lotQuery.data?.lot?.status ?? 0) // DRAFT, PENDING_QC, QC_REJECTED
  );
</script>

<div class="max-w-3xl space-y-6">
  {#if $lotQuery.isLoading}
    <p class="text-gray-500">Loading...</p>
  {:else if $lotQuery.isError}
    <p class="text-red-500">Failed to load lot</p>
  {:else if $lotQuery.data?.lot}
    {@const lot = $lotQuery.data.lot}
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold">{lot.lotNumber}</h1>
        <p class="text-gray-500 text-sm">{lot.materialName} — {lot.supplierName}</p>
      </div>
      <span class="px-3 py-1 rounded-full text-sm font-medium bg-blue-100 text-blue-700">
        {statusLabels[lot.status] ?? 'Unknown'}
      </span>
    </div>

    <div class="grid grid-cols-2 gap-6">
      <div class="border rounded-lg p-4 space-y-3">
        <h2 class="font-semibold text-sm text-gray-500 uppercase">Lot Details</h2>
        <dl class="space-y-2 text-sm">
          <div class="flex justify-between"><dt class="text-gray-500">Material Type</dt><dd>{materialLabels[lot.materialType]}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Quantity</dt><dd>{lot.quantity} {lot.unit}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Arrival Date</dt><dd>{lot.arrivalDate}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Created By</dt><dd>{lot.createdBy}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Created</dt><dd>{lot.createdAt?.toDate().toLocaleString()}</dd></div>
        </dl>
      </div>

      <div class="border rounded-lg p-4 space-y-3">
        <h2 class="font-semibold text-sm text-gray-500 uppercase">Storage Requirement</h2>
        <dl class="space-y-2 text-sm">
          <div class="flex justify-between"><dt class="text-gray-500">Temperature</dt><dd>{tempLabels[lot.storageRequirement?.temperatureRange ?? 0] ?? '—'}</dd></div>
          <div class="flex justify-between"><dt class="text-gray-500">Hazard/Drum Class</dt><dd>{hazardLabels[lot.storageRequirement?.hazardClass ?? 0] ?? '—'}</dd></div>
        </dl>
      </div>
    </div>

    <!-- QC Image Upload -->
    <div class="border rounded-lg p-4 space-y-3">
      <h2 class="font-semibold text-sm text-gray-500 uppercase">QC Image Upload</h2>
      {#if uploadedKey}
        <div class="flex items-center gap-2 text-green-700 bg-green-50 p-3 rounded">
          <span>✓</span>
          <span class="text-sm">Uploaded: <code class="text-xs">{uploadedKey}</code></span>
        </div>
        <!-- Start QC button -->
        {#if !qcStarted}
          <button
            onclick={handleStartQC}
            disabled={startingQC}
            class="px-4 py-2 bg-orange-600 text-white rounded-md hover:bg-orange-700 text-sm disabled:opacity-50"
          >
            {startingQC ? 'Starting QC...' : '🔬 Start QC'}
          </button>
          {#if qcError}
            <p class="text-sm text-red-600">{qcError}</p>
          {/if}
        {:else}
          <div class="flex items-center gap-2 text-blue-700 bg-blue-50 p-3 rounded">
            <span>🔬</span>
            <span class="text-sm">QC job created — lot advanced to QC Review</span>
          </div>
        {/if}
      {:else if uploadAllowed}
        <div class="space-y-2">
          <input
            type="file"
            accept="image/jpeg,image/png"
            onchange={handleUpload}
            disabled={uploading}
            class="block w-full text-sm text-gray-500 file:mr-4 file:py-2 file:px-4 file:rounded file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100"
          />
          {#if uploading}
            <div class="w-full bg-gray-200 rounded-full h-2">
              <div class="bg-blue-600 h-2 rounded-full transition-all" style="width: {uploadProgress}%"></div>
            </div>
            <p class="text-xs text-gray-500">{uploadProgress}%</p>
          {/if}
          {#if uploadError}
            <p class="text-sm text-red-600">{uploadError}</p>
          {/if}
        </div>
      {:else}
        <p class="text-sm text-gray-400">Upload not available in current status.</p>
      {/if}
    </div>

    <!-- Timeline -->
    <div class="border rounded-lg p-4">
      <h2 class="font-semibold text-sm text-gray-500 uppercase mb-3">Timeline</h2>
      {#if $timelineQuery.isLoading}
        <p class="text-gray-400 text-sm">Loading timeline...</p>
      {:else if ($timelineQuery.data?.entries?.length ?? 0) > 0}
        <div class="space-y-3">
          {#each $timelineQuery.data?.entries ?? [] as entry}
            <div class="flex gap-3 text-sm">
              <div class="w-2 h-2 rounded-full bg-blue-400 mt-1.5 shrink-0"></div>
              <div>
                <span class="font-medium">{entry.action}</span>
                <span class="text-gray-400 ml-2">by {entry.actorUserId} ({entry.actorRole})</span>
                <p class="text-xs text-gray-400">{entry.createdAt?.toDate().toLocaleString()}</p>
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <p class="text-gray-400 text-sm">No audit entries yet.</p>
      {/if}
    </div>

    <a href="/lots" class="inline-block text-sm text-blue-600 hover:underline">← Back to lots</a>
  {/if}
</div>
