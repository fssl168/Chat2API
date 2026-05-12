import axios from 'axios';
import type { Provider, Account, ApiResponse, BrowserTask, TokenValidationResult } from './types';

const API_BASE = 'http://localhost:8080/api/v1';

const api = axios.create({
  baseURL: API_BASE,
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
});

export async function getProviders(): Promise<ApiResponse<Provider[]>> {
  const { data } = await api.get('/providers');
  return data;
}

export async function getAccounts(providerType: string): Promise<ApiResponse<Account[]>> {
  const { data } = await api.get(`/providers/${providerType}/accounts`);
  return data;
}

export async function addAccount(providerType: string, account: Partial<Account>): Promise<ApiResponse<Account>> {
  const { data } = await api.post(`/providers/${providerType}/accounts`, account);
  return data;
}

export async function updateAccount(providerType: string, accountId: string, updates: Partial<Account>): Promise<ApiResponse<Account>> {
  const { data } = await api.put(`/providers/${providerType}/accounts/${accountId}`, updates);
  return data;
}

export async function deleteAccount(providerType: string, accountId: string): Promise<ApiResponse<void>> {
  const { data } = await api.delete(`/providers/${providerType}/accounts/${accountId}`);
  return data;
}

export async function validateAccount(providerType: string, accountId: string): Promise<ApiResponse<TokenValidationResult & { account?: Account }>> {
  const { data } = await api.post(`/providers/${providerType}/accounts/${accountId}/validate`);
  return data;
}

export async function loginWithBrowser(req: {
  providerId: string;
  providerType: string;
  timeout?: number;
}): Promise<ApiResponse<{ taskId: string }>> {
  const { data } = await api.post('/oauth/login/browser', req);
  return data;
}

export async function pollBrowserTask(
  taskId: string,
  onStatusUpdate?: (status: BrowserTask) => void
): Promise<BrowserTask> {
  const maxAttempts = 300;
  const interval = 2000;
  const maxConsecutiveErrors = 5;
  let consecutiveErrors = 0;

  for (let i = 0; i < maxAttempts; i++) {
    try {
      const { data: resp, status } = await api.get(`/oauth/browser/tasks/${taskId}`, {
        validateStatus: () => true,
      });

      if (status === 404) {
        return { id: taskId, status: 'failed', result: { error: 'Task not found (server may have restarted)' } };
      }

      if (status !== 200) {
        consecutiveErrors++;
        if (consecutiveErrors >= maxConsecutiveErrors) {
          return { id: taskId, status: 'failed', result: { error: `Server error (HTTP ${status})` } };
        }
        await new Promise(resolve => setTimeout(resolve, interval));
        continue;
      }

      consecutiveErrors = 0;
      const task: BrowserTask = resp.data || resp;

      if (onStatusUpdate) {
        onStatusUpdate(task);
      }

      if (task.status === 'completed' || task.status === 'failed') {
        return task;
      }
    } catch (err) {
      consecutiveErrors++;
      if (consecutiveErrors >= maxConsecutiveErrors) {
        return { id: taskId, status: 'failed', result: { error: 'Connection lost - server may be down' } };
      }
      console.error('[pollBrowserTask] Error:', err);
    }

    await new Promise(resolve => setTimeout(resolve, interval));
  }

  return { id: taskId, status: 'failed', result: { error: 'Polling timeout' } };
}

export async function validateToken(req: {
  providerId: string;
  providerType: string;
  credentials: Record<string, string>;
}): Promise<ApiResponse<TokenValidationResult>> {
  const { data } = await api.post('/oauth/validate', req);
  return data;
}
