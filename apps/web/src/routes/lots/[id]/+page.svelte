<script lang="ts">
  import { page } from '$app/stores';
  import { t } from 'svelte-i18n';
  import { get } from 'svelte/store';
  import { createQuery, getQueryClientContext } from '@tanstack/svelte-query';
  import { createClient } from '@connectrpc/connect';
  import { transport } from '$lib/connect';
  import { LotService } from '$lib/gen/simaops/lot/v1/lot_pb';
  import { QCService } from '$lib/gen/simaops/qc/v1/qc_pb';
  import { WarehouseService } from '$lib/gen/simaops/warehouse/v1/warehouse_pb';

  const lotClient = createClient(LotService, transport);
  const qcClient = createClient(QCService, transport);
  const whClient = createClient(WarehouseService, transport);
  const queryClient = getQueryClientContext();

  const lotId = $derived($page.params.id);

  const lotQuery = createQuery(() => ({
    queryKey: ['lot', lotId],
    queryFn: () => lotClient.getLot({ lotId }),
    enabled: !!lotId,
  }));

  const timelineQuery = createQuery(() => ({
    queryKey: ['lot-timeline', lotId],
    // Use the lot-scoped LotService/GetLotTimeline (open to any authenticated
    // user) rather than AuditService/GetEntityAuditTrail (MANAGER/ADMIN only).
    // The lot detail page is reachable by every role, so the audit-service
    // call 403'd for OPERATOR/QC/WAREHOUSE users. Both return the same
    // TimelineEntry shape (action/actorUserId/actorRole/createdAt).
    queryFn: () => lotClient.getLotTimeline({ lotId }),
    enabled: !!lotId,
  }));

  let uploadProgress = $state(0);
  let uploading = $state(false);
  let uploadedKey = $state('');
  let uploadError = $state('');

  async function handleUpload(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;

    if (!file.type.startsWith('image/')) {
      uploadError = get(t)('qc.upload_image_only');
      return;
    }
    if (file.size > 10 * 1024 * 1024) {
      uploadError = get(t)('qc.upload_too_large');
      return;
    }

    uploading = true;
    uploadProgress = 0;
    uploadError = '';

    try {
      const res = await qcClient.createQCUploadUrl({
        lotId,
        filename: file.name,
        contentType: file.type || 'image/jpeg',
        idempotencyKey: crypto.randomUUID()
      });

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
        xhr.onerror = () => reject(new Error(get(t)('qc.upload_network_error')));
        xhr.send(file);
      });
    } catch (err: any) {
      uploadError = err.message || get(t)('qc.upload_failed');
    } finally {
      uploading = false;
    }
  }

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
      queryClient.invalidateQueries({ queryKey: ['lot', lotId] });
      queryClient.invalidateQueries({ queryKey: ['lot-timeline', lotId] });
    } catch (e: any) {
      qcError = e.message || get(t)('common.error');
    } finally {
      startingQC = false;
    }
  }

  // Statuses where uploading (or re-uploading) a QC image is allowed:
  // DRAFT(1), PENDING_QC(2), QC_REVIEW(4 — supervisor asked for a recheck or
  // operator wants a better image), QC_REJECTED(6). A new image supersedes any
  // in-flight QC job for the lot.
  const uploadAllowed = $derived(
    [1, 2, 4, 6].includes(lotQuery.data?.lot?.status ?? 0)
  );
  // True when a QC cycle already happened, so the upload is a replacement.
  const isResubmit = $derived(
    [4, 6].includes(lotQuery.data?.lot?.status ?? 0)
  );

  // Warehouse slot decisions query (only for lots with warehouse assignment)
  const slotDecisionsQuery = createQuery(() => ({
    queryKey: ['slot-decisions', lotId],
    queryFn: () => whClient.listSlotDecisions({ lotId }),
    enabled: !!lotId && [7, 8].includes(lotQuery.data?.lot?.status ?? 0),
  }));

  // Unassign functionality
  let showUnassignModal = $state(false);
  let unassignReason = $state('');
  let unassigning = $state(false);
  let unassignError = $state('');

  async function openUnassign() {
    unassignReason = '';
    unassignError = '';
    showUnassignModal = true;
  }

  async function doUnassign() {
    if (!unassignReason.trim()) {
      unassignError = 'Please provide a reason';
      return;
    }
    unassigning = true;
    unassignError = '';
    try {
      await whClient.unassignSlot({
        lotId,
        reason: unassignReason,
        idempotencyKey: crypto.randomUUID(),
      });
      showUnassignModal = false;
      queryClient.invalidateQueries({ queryKey: ['lot', lotId] });
      queryClient.invalidateQueries({ queryKey: ['slot-decisions', lotId] });
    } catch (e: any) {
      unassignError = e.message || 'Unassign failed';
    } finally {
      unassigning = false;
    }
  }
</script>

<div class="max-w-3xl space-y-6">
  {#if lotQuery.isLoading}
    <p class="text-slate-500">{$t('common.loading')}</p>
  {:else if lotQuery.isError}
    <div class="p-3 bg-red-50 border border-red-200 rounded text-red-700 text-sm">
      {lotQuery.error?.message || $t('common.error')}
    </div>
  {:else if lotQuery.data?.lot}
    {@const lot = lotQuery.data.lot}
    <div class="flex items-center justify-between">
      <div>
        <a href="/lots" class="text-sm text-slate-500 transition-colors hover:text-blue-600">{$t('lot.back_to_lots')}</a>
        <h1 class="text-2xl font-bold tracking-normal text-slate-950 mt-1">{lot.lotNumber}</h1>
        <p class="text-slate-500 text-sm">{lot.materialName} — {lot.supplierName}</p>
      </div>
      <span class="rounded-md px-2.5 py-1 text-xs font-semibold bg-blue-100 text-blue-700 ring-1 ring-inset ring-blue-200">
        {$t(`lot_status.${lot.status}`)}
      </span>
    </div>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
      <div class="border border-slate-200 shadow-sm rounded-lg p-4 space-y-3 bg-white">
        <h2 class="font-semibold text-sm text-slate-500 uppercase tracking-normal">{$t('lot.details')}</h2>
        <dl class="space-y-2 text-sm">
          <div class="flex justify-between"><dt class="text-slate-500">{$t('lot.material_type')}</dt><dd class="text-slate-950">{$t(`material_type.${lot.materialType}`)}</dd></div>
          <div class="flex justify-between"><dt class="text-slate-500">{$t('lot.quantity')}</dt><dd class="text-slate-950">{lot.quantity} {lot.unit}</dd></div>
          <div class="flex justify-between"><dt class="text-slate-500">{$t('lot.arrival_date')}</dt><dd class="text-slate-950">{lot.arrivalDate}</dd></div>
          <div class="flex justify-between"><dt class="text-slate-500">{$t('lot.created_by')}</dt><dd class="text-slate-950">{lot.createdBy}</dd></div>
          <div class="flex justify-between"><dt class="text-slate-500">{$t('common.created')}</dt><dd class="text-slate-950">{lot.createdAt ? new Date(Number(lot.createdAt.seconds) * 1000).toLocaleString() : ''}</dd></div>
        </dl>
      </div>

      <div class="border border-slate-200 shadow-sm rounded-lg p-4 space-y-3 bg-white">
        <h2 class="font-semibold text-sm text-slate-500 uppercase tracking-normal">{$t('lot.storage_requirement')}</h2>
        <dl class="space-y-2 text-sm">
          <div class="flex justify-between"><dt class="text-slate-500">{$t('lot.temperature_range')}</dt><dd class="text-slate-950">{lot.storageRequirement?.temperatureRange ? $t(`temp_range.${lot.storageRequirement.temperatureRange}`) : '—'}</dd></div>
          <div class="flex justify-between"><dt class="text-slate-500">{$t('lot.hazard_class')}</dt><dd class="text-slate-950">{lot.storageRequirement?.hazardClass ? $t(`hazard.${lot.storageRequirement.hazardClass}`) : '—'}</dd></div>
        </dl>
      </div>

      <!-- Warehouse Assignment Section (only show for assigned/ready lots) -->
      {#if [7, 8].includes(lot.status)}
        <div class="border border-slate-200 shadow-sm rounded-lg p-4 space-y-3 bg-white md:col-span-2">
          <div class="flex items-center justify-between">
            <h2 class="font-semibold text-sm text-slate-500 uppercase tracking-normal">{$t('lot.warehouse_assignment')}</h2>
            <div class="flex items-center gap-2">
              <span class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-semibold {lot.assignment?.decisionType === 1 ? 'bg-blue-100 text-blue-700' : lot.assignment?.decisionType === 3 ? 'bg-amber-100 text-amber-700' : 'bg-slate-100 text-slate-700'}">
                {#if lot.assignment?.decisionType === 1}
                  <svg viewBox="0 0 24 24" class="size-3" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm0 18a8 8 0 1 1 8-8 8 8 0 0 1-8 8z"/><path d="M12 6v6l4 2"/></svg>
                  Auto-assigned
                {:else if lot.assignment?.decisionType === 3}
                  <svg viewBox="0 0 24 24" class="size-3" fill="none" stroke="currentColor" stroke-width="2"><path d="M1 4v6h6M23 20v-6h-6"/><path d="M20.49 9A9 9 0 0 0 5.64 5.64L1 4m22 6-4.64 4.36A9 9 0 0 1 3.51 15"/></svg>
                  Override
                {:else}
                  Manual
                {/if}
              </span>
            </div>
          </div>

          <dl class="space-y-2 text-sm">
            <div class="flex justify-between">
              <dt class="text-slate-500">Slot</dt>
              <dd class="text-slate-950 font-mono font-semibold">{lot.assignment?.locationCode ?? '—'}</dd>
            </div>
            <div class="flex justify-between">
              <dt class="text-slate-500">Assigned by</dt>
              <dd class="text-slate-950">{lot.assignment?.assignedBy ?? '—'}</dd>
            </div>
            {#if lot.assignment?.assignedAt}
              <div class="flex justify-between">
                <dt class="text-slate-500">Assigned at</dt>
                <dd class="text-slate-950">{new Date(Number(lot.assignment.assignedAt.seconds) * 1000).toLocaleString()}</dd>
              </div>
            {/if}
            {#if lot.assignment?.reason}
              <div class="flex justify-between">
                <dt class="text-slate-500">Reason</dt>
                <dd class="text-slate-950">{lot.assignment.reason}</dd>
              </div>
            {/if}
          </dl>

          <!-- Slot Decision History -->
          {#if slotDecisionsQuery.data?.decisions?.length}
            <div class="mt-4 pt-4 border-t border-slate-100">
              <h3 class="text-xs font-semibold text-slate-500 uppercase tracking-normal mb-2">Decision History</h3>
              <div class="space-y-2">
                {#each slotDecisionsQuery.data.decisions as decision}
                  <div class="flex items-start gap-2 text-xs">
                    <div class="w-2 h-2 rounded-full mt-1 shrink-0 {decision.decisionType === 1 ? 'bg-blue-400' : decision.decisionType === 3 ? 'bg-amber-400' : 'bg-slate-400'}"></div>
                    <div class="flex-1">
                      <span class="font-medium">{decision.decisionType === 1 ? 'Auto-assigned' : decision.decisionType === 3 ? 'Override' : 'Manual'}</span>
                      to <span class="font-mono">{decision.locationCode}</span>
                      by {decision.actorId}
                      {#if decision.reason}
                        — {decision.reason}
                      {/if}
                    </div>
                    <span class="text-slate-400 shrink-0">{new Date(Number(decision.createdAt.seconds) * 1000).toLocaleString()}</span>
                  </div>
                {/each}
              </div>
            </div>
          {/if}

          <!-- Unassign Button -->
          {#if [7, 8].includes(lot.status)}
            <div class="mt-4 pt-4 border-t border-slate-100 flex justify-end">
              <button
                onclick={openUnassign}
                class="inline-flex h-8 items-center gap-1.5 rounded-md border border-red-200 bg-white px-3 text-xs font-semibold text-red-600 shadow-sm transition-colors hover:bg-red-50"
              >
                <svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 6h18M8 6V4h8v2M19 6v14a2 2 0 01-2 2H7a2 2 0 01-2-2V6"/></svg>
                Release Slot
              </button>
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- QC Image Upload -->
    <div class="border border-slate-200 shadow-sm rounded-lg p-4 space-y-3 bg-white">
      <h2 class="font-semibold text-sm text-slate-500 uppercase tracking-normal">{$t('qc.image_upload')}</h2>
      {#if uploadedKey}
        <div class="flex items-center gap-2 text-emerald-700 bg-emerald-50 p-3 rounded-md">
          <span aria-hidden="true">✓</span>
          <span class="text-sm">{$t('qc.image_uploaded')}: <code class="font-mono text-xs break-all">{uploadedKey}</code></span>
        </div>
        {#if !qcStarted}
          <button
            onclick={handleStartQC}
            disabled={startingQC}
            class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 text-sm font-semibold disabled:opacity-50"
          >
            {startingQC ? $t('qc.starting_qc') : $t('qc.start_qc')}
          </button>
          {#if qcError}
            <p class="text-sm text-red-600">{qcError}</p>
          {/if}
        {:else}
          <div class="flex items-center gap-2 text-blue-700 bg-blue-50 p-3 rounded-md">
            <span aria-hidden="true">🔬</span>
            <span class="text-sm">{$t('qc.qc_started')}</span>
          </div>
        {/if}
      {:else if uploadAllowed}
        <div class="space-y-2">
          {#if isResubmit}
            <p class="text-xs text-amber-700 bg-amber-50 border border-amber-200 rounded-md px-2 py-1.5">
              {$t('qc.upload_replaces')}
            </p>
          {/if}
          <input
            type="file"
            accept="image/jpeg,image/png"
            onchange={handleUpload}
            disabled={uploading}
            aria-label="Upload QC image"
            class="block w-full text-sm text-slate-500 file:mr-4 file:py-2 file:px-4 file:rounded-md file:border-0 file:text-sm file:font-medium file:bg-blue-50 file:text-blue-700 hover:file:bg-blue-100"
          />
          {#if uploading}
            <div class="w-full bg-slate-200 rounded-full h-2" role="progressbar" aria-valuenow={uploadProgress} aria-valuemin="0" aria-valuemax="100">
              <div class="bg-blue-600 h-2 rounded-full transition-all" style="width: {uploadProgress}%"></div>
            </div>
            <p class="text-xs text-slate-500">{uploadProgress}%</p>
          {/if}
          {#if uploadError}
            <p class="text-sm text-red-600">{uploadError}</p>
          {/if}
        </div>
      {:else}
        <p class="text-sm text-slate-400">
          {#if lot.status === 3}
            {$t('qc.upload_processing')}
          {:else if [5, 7, 8].includes(lot.status)}
            {$t('qc.upload_done')}
          {:else}
            {$t('qc.upload_unavailable')}
          {/if}
        </p>
      {/if}
    </div>

    <!-- Timeline -->
    <div class="border border-slate-200 shadow-sm rounded-lg p-4 bg-white">
      <h2 class="font-semibold text-sm text-slate-500 uppercase tracking-normal mb-3">{$t('common.timeline')}</h2>
      {#if timelineQuery.isLoading}
        <p class="text-slate-400 text-sm">{$t('lot.loading_timeline')}</p>
      {:else if (timelineQuery.data?.entries?.length ?? 0) > 0}
        <div class="space-y-3">
          {#each timelineQuery.data?.entries ?? [] as entry}
            <div class="flex gap-3 text-sm">
              <div class="w-2 h-2 rounded-full bg-blue-400 mt-1.5 shrink-0" aria-hidden="true"></div>
              <div>
                <span class="font-medium font-mono text-xs text-slate-950">{entry.action}</span>
                <span class="text-slate-500 ml-2">by {entry.actorUserId} ({entry.actorRole})</span>
                <p class="text-xs text-slate-400">{entry.createdAt ? new Date(Number(entry.createdAt.seconds) * 1000).toLocaleString() : ''}</p>
              </div>
            </div>
          {/each}
        </div>
      {:else}
        <p class="text-slate-400 text-sm">{$t('lot.no_timeline')}</p>
      {/if}
    </div>

    <!-- Unassign Modal -->
    {#if showUnassignModal}
      <div
        class="fixed inset-0 z-50 flex items-center justify-center bg-slate-950/55 p-4"
        role="dialog"
        aria-modal="true"
        aria-labelledby="unassign-title"
        onclick={(event) => {
          if (event.target === event.currentTarget) showUnassignModal = false;
        }}
      >
        <div class="w-full max-w-md overflow-hidden rounded-lg border border-slate-200 bg-white shadow-xl">
          <div class="border-b border-slate-200 px-5 py-4">
            <p class="text-xs font-semibold uppercase tracking-normal text-red-600">Release Slot Assignment</p>
            <h2 id="unassign-title" class="mt-1 text-lg font-bold text-slate-950">Release Slot for {lot.lotNumber}</h2>
          </div>
          <div class="px-5 py-4">
            <p class="mb-4 text-sm text-slate-600">
              This will release the slot and move the lot back to QC_APPROVED status.
            </p>
            <label for="unassign-reason" class="block text-sm font-medium text-slate-700">
              Reason <span class="text-red-500">*</span>
            </label>
            <textarea
              id="unassign-reason"
              bind:value={unassignReason}
              rows="3"
              placeholder="Why are you releasing this slot?"
              class="mt-1 block w-full rounded-md border border-slate-300 px-3 py-2 text-sm shadow-sm focus:border-purple-500 focus:outline-none focus:ring-1 focus:ring-purple-500"
            ></textarea>
            {#if unassignError}
              <p class="mt-2 text-sm text-red-600" role="alert">{unassignError}</p>
            {/if}
          </div>
          <div class="flex justify-end gap-3 border-t border-slate-200 bg-slate-50 px-5 py-3">
            <button
              onclick={() => showUnassignModal = false}
              class="h-9 rounded-md border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 shadow-sm hover:bg-slate-50"
            >
              Cancel
            </button>
            <button
              onclick={doUnassign}
              disabled={unassigning || !unassignReason.trim()}
              class="inline-flex h-9 items-center gap-2 rounded-md bg-red-600 px-4 text-sm font-semibold text-white shadow-sm transition-colors hover:bg-red-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {unassigning ? 'Releasing...' : 'Release Slot'}
            </button>
          </div>
        </div>
      </div>
    {/if}
  {/if}
</div>
