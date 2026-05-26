import { createConnectTransport } from '@connectrpc/connect-web';
import { browser } from '$app/environment';

// In production: API is at api.<same-ip>.sslip.io
// In dev: API is at localhost:8080
function getApiUrl(): string {
  if (!browser) return 'http://simaops-api.simaops:8080'; // SSR: internal cluster URL
  const host = window.location.hostname;
  if (host.includes('sslip.io')) {
    return `http://api.${host.replace('app.', '')}`;
  }
  return 'http://localhost:8080';
}

export const transport = createConnectTransport({
  baseUrl: getApiUrl(),
  useBinaryFormat: false
});
