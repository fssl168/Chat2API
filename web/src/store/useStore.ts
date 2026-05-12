import { create } from 'zustand';
import type { Provider, Account, ProviderType } from '../api/types';
import * as api from '../api/client';

interface AppState {
  providers: Provider[];
  accounts: Record<string, Account[]>;
  selectedProvider: ProviderType | null;
  isLoading: boolean;
  error: string | null;

  loadProviders: () => Promise<void>;
  loadAccounts: (providerType: string) => Promise<void>;
  selectProvider: (providerType: ProviderType | null) => void;
  addAccount: (providerType: ProviderType, account: Account) => Promise<void>;
  removeAccount: (providerType: ProviderType, accountId: string) => Promise<void>;
  updateAccount: (providerType: ProviderType, accountId: string, updates: Partial<Account>) => Promise<void>;
  setError: (error: string | null) => void;
}

export const useAppStore = create<AppState>((set, get) => ({
  providers: [],
  accounts: {},
  selectedProvider: null,
  isLoading: false,
  error: null,

  loadProviders: async () => {
    set({ isLoading: true, error: null });
    try {
      const result = await api.getProviders();
      if (result.success && result.data) {
        set({ providers: result.data, isLoading: false });
      } else {
        set({ error: result.error || 'Failed to load providers', isLoading: false });
      }
    } catch (err) {
      set({ error: err instanceof Error ? err.message : 'Failed to load providers', isLoading: false });
    }
  },

  loadAccounts: async (providerType: string) => {
    try {
      const result = await api.getAccounts(providerType);
      if (result.success && result.data) {
        set(state => ({
          accounts: { ...state.accounts, [providerType]: result.data! },
        }));
      }
    } catch (err) {
      console.error('Failed to load accounts:', err);
    }
  },

  selectProvider: (providerType) => {
    set({ selectedProvider: providerType });
    if (providerType) {
      get().loadAccounts(providerType);
    }
  },

  addAccount: async (providerType, account) => {
    try {
      const result = await api.addAccount(providerType, account);
      if (result.success) {
        await get().loadAccounts(providerType);
      }
    } catch (err) {
      console.error('Failed to add account:', err);
    }
  },

  removeAccount: async (providerType, accountId) => {
    try {
      await api.deleteAccount(providerType, accountId);
      set(state => ({
        accounts: {
          ...state.accounts,
          [providerType]: (state.accounts[providerType] || []).filter(a => a.id !== accountId),
        },
      }));
    } catch (err) {
      console.error('Failed to delete account:', err);
    }
  },

  updateAccount: async (providerType, accountId, updates) => {
    try {
      await api.updateAccount(providerType, accountId, updates);
      await get().loadAccounts(providerType);
    } catch (err) {
      console.error('Failed to update account:', err);
    }
  },

  setError: (error) => set({ error }),
}));
