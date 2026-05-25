import { createConnectTransport } from '@connectrpc/connect-web';

const API_BASE_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080';

export const transport = createConnectTransport({
  baseUrl: API_BASE_URL,
  // Connect protocol over JSON for browser debugging
  useBinaryFormat: false
});
