import { createConnectTransport } from '@connectrpc/connect-web';
import { browser } from '$app/environment';

function getApiUrl(): string {
  if (!browser) return 'http://simaops-api.simaops:8080';
  const host = window.location.hostname;
  if (host.includes('sslip.io')) {
    const proto = window.location.protocol; // 'https:' or 'http:'
    return `${proto}//api.${host.replace('app.', '')}`;
  }
  return 'http://localhost:8080';
}

function getCookie(name: string): string | undefined {
  if (!browser) return undefined;
  const match = document.cookie.match(new RegExp(`(?:^|; )${name}=([^;]*)`));
  return match ? decodeURIComponent(match[1]) : undefined;
}

export const transport = createConnectTransport({
  baseUrl: getApiUrl(),
  useBinaryFormat: false,
  interceptors: [
    (next) => async (req) => {
      const token = getCookie('sa_access');
      if (token) {
        req.header.set('Authorization', `Bearer ${token}`);
      }
      return next(req);
    }
  ]
});
