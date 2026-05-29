<script lang="ts">
  import { t } from 'svelte-i18n';
  import { popupLogin } from '$lib/auth/popupLogin';
  import { focusTrap } from '$lib/actions/focusTrap.svelte';

  let { open = $bindable(false), onSignedIn, onSignOut }: {
    open?: boolean;
    onSignedIn?: () => void;
    onSignOut?: () => void;
  } = $props();

  let attempting = $state(false);
  let attemptError = $state('');

  async function handleSignIn() {
    if (attempting) return;
    attempting = true;
    attemptError = '';
    try {
      const ok = await popupLogin();
      if (ok) {
        open = false;
        onSignedIn?.();
      } else {
        // Popup blocked or user closed it. Fall back to a full redirect so
        // the user has *some* path forward.
        const returnTo = encodeURIComponent(window.location.pathname + window.location.search);
        window.location.assign(`/auth/login?return_to=${returnTo}`);
      }
    } catch (e) {
      attemptError = (e as Error)?.message ?? 'Sign-in failed';
    } finally {
      attempting = false;
    }
  }

  function handleSignOut() {
    onSignOut?.();
    window.location.assign('/auth/logout');
  }
</script>

{#if open}
  <div
    class="fixed inset-0 bg-black/50 flex items-center justify-center z-[60] p-4"
    role="dialog"
    aria-modal="true"
    aria-labelledby="session-expired-title"
    aria-describedby="session-expired-body"
    use:focusTrap
  >
    <div class="bg-white rounded-lg p-6 w-full max-w-md space-y-4 shadow-xl">
      <div class="flex items-center gap-3">
        <span class="text-3xl" aria-hidden="true">🔒</span>
        <h2 id="session-expired-title" class="text-lg font-bold">
          {$t('auth.session_expired.title')}
        </h2>
      </div>
      <p id="session-expired-body" class="text-sm text-gray-600">
        {$t('auth.session_expired.body')}
      </p>
      {#if attemptError}
        <p class="text-sm text-red-600" role="alert">{attemptError}</p>
      {/if}
      <div class="flex gap-3 justify-end">
        <button
          onclick={handleSignOut}
          class="px-4 py-2 border border-gray-300 rounded-md text-sm hover:bg-gray-50"
          disabled={attempting}
        >
          {$t('auth.session_expired.sign_out')}
        </button>
        <button
          onclick={handleSignIn}
          disabled={attempting}
          class="px-4 py-2 bg-blue-600 text-white rounded-md text-sm hover:bg-blue-700 disabled:opacity-50"
        >
          {attempting ? $t('common.loading') : $t('auth.session_expired.sign_in')}
        </button>
      </div>
    </div>
  </div>
{/if}
