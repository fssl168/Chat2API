import { useState, useEffect } from 'react';
import { useAppStore } from './store/useStore';
import { ProviderList } from './components/ProviderList';
import { AccountList } from './components/AccountCard';
import { AddAccountDialog } from './components/AddAccountDialog';
import { validateAccount as apiValidateAccount } from './api/client';
import type { Account, ProviderType } from './api/types';

type ViewMode = 'providers' | 'accounts' | 'account-detail';

const providerIcons: Record<string, string> = {
  deepseek: '🔍', glm: '🧠', kimi: '🌙', minimax: '⚡',
  qwen: '☁️', 'qwen-ai': '🌐', zai: '🤖', perplexity: '🔮', mimo: '📱',
};

export default function App() {
  const { providers, accounts, selectedProvider, loadProviders, selectProvider, removeAccount, updateAccount, loadAccounts } = useAppStore();
  const [viewMode, setViewMode] = useState<ViewMode>('providers');
  const [showAddDialog, setShowAddDialog] = useState(false);
  const [editingAccount, setEditingAccount] = useState<Account | null>(null);
  const [detailAccount, setDetailAccount] = useState<Account | null>(null);
  const [successMessage, setSuccessMessage] = useState('');
  const [error, setError] = useState('');

  useEffect(() => { loadProviders(); }, []);

  const selectedProviderInfo = providers.find(p => p.providerType === selectedProvider);
  const selectedAccounts = selectedProvider ? (accounts[selectedProvider] || []) : [];

  const handleSelectProvider = (type: ProviderType) => {
    selectProvider(type);
    setViewMode('accounts');
  };

  const handleAddAccount = async (account: Account) => {
    if (!selectedProvider) return;
    try {
      await useAppStore.getState().addAccount(selectedProvider, account);
      setShowAddDialog(false);
      setSuccessMessage(`Account "${account.name}" added successfully!`);
      setTimeout(() => setSuccessMessage(''), 3000);
    } catch (err) {
      setError(`Failed to add account: ${err instanceof Error ? err.message : 'Unknown error'}`);
      setTimeout(() => setError(''), 5000);
    }
  };

  const handleEditAccount = (account: Account) => {
    setEditingAccount(account);
    setShowAddDialog(true);
  };

  const handleDeleteAccount = async (id: string) => {
    if (!selectedProvider) return;
    try {
      await removeAccount(selectedProvider as ProviderType, id);
      if (detailAccount?.id === id) {
        setDetailAccount(null);
        setViewMode('accounts');
      }
      setSuccessMessage('Account deleted successfully!');
      setTimeout(() => setSuccessMessage(''), 3000);
    } catch (err) {
      setError(`Delete failed: ${err instanceof Error ? err.message : 'Unknown error'}`);
      setTimeout(() => setError(''), 5000);
    }
  };

  const handleValidateAccount = async (id: string) => {
    if (!selectedProvider) return;
    try {
      const result = await apiValidateAccount(selectedProvider, id);
      await loadAccounts(selectedProvider);
      if (result.success) {
        setSuccessMessage('Account validated successfully!');
      } else {
        setError(`Validation failed: ${result.data?.error || 'Unknown error'}`);
      }
    } catch (err) {
      setError(`Validation error: ${err instanceof Error ? err.message : 'Unknown error'}`);
    }
    setTimeout(() => { setSuccessMessage(''); setError(''); }, 5000);
  };

  const navigateBack = () => {
    if (viewMode === 'account-detail') {
      setViewMode('accounts');
      setDetailAccount(null);
    } else {
      setViewMode('providers');
      selectProvider(null);
    }
  };

  return (
    <div className="min-h-screen bg-gradient-to-br from-slate-50 via-blue-50/30 to-indigo-50/20">
      <header className="sticky top-0 z-40 bg-white/80 backdrop-blur-xl border-b border-slate-200/60">
        <div className="max-w-7xl mx-auto px-6 py-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              {viewMode !== 'providers' && (
                <button
                  onClick={navigateBack}
                  className="p-2 hover:bg-slate-100 rounded-xl transition-colors"
                >
                  <svg className="w-5 h-5 text-slate-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
                  </svg>
                </button>
              )}
              <div className="flex items-center gap-2">
                {viewMode !== 'providers' && selectedProviderInfo && (
                  <span className="text-xl">{providerIcons[selectedProviderInfo.providerType] || '📦'}</span>
                )}
                <div>
                  <h1 className="text-xl font-bold text-slate-800">
                    {viewMode === 'providers' ? 'Chat2API Manager' :
                     viewMode === 'accounts' ? selectedProviderInfo?.label || 'Accounts' :
                     detailAccount?.name || 'Detail'}
                  </h1>
                  <p className="text-sm text-slate-500">
                    {viewMode === 'providers' ? 'Manage AI service providers and accounts' :
                     viewMode === 'accounts' ? `${selectedAccounts.length} account(s) • ${selectedProviderInfo?.authType || ''}` :
                     'Account details and statistics'}
                  </p>
                </div>
              </div>
            </div>
            {viewMode === 'accounts' && (
              <button
                onClick={() => { setEditingAccount(null); setShowAddDialog(true); }}
                className="px-4 py-2.5 bg-gradient-to-r from-indigo-600 to-indigo-700 text-white rounded-xl hover:from-indigo-700 hover:to-indigo-800 transition-all font-medium text-sm flex items-center gap-2 shadow-sm"
              >
                <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                Add Account
              </button>
            )}
          </div>
        </div>
      </header>

      {successMessage && (
        <div className="max-w-7xl mx-auto px-6 mt-4">
          <div className="bg-emerald-50 border border-emerald-200 text-emerald-700 px-4 py-3 rounded-xl text-sm flex items-center gap-2">
            <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
            {successMessage}
          </div>
        </div>
      )}
      {error && (
        <div className="max-w-7xl mx-auto px-6 mt-4">
          <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-xl text-sm flex items-center gap-2">
            <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
            {error}
          </div>
        </div>
      )}

      <main className="max-w-7xl mx-auto px-6 py-6">
        {viewMode === 'providers' && (
          <ProviderList
            providers={providers}
            accounts={accounts}
            onSelect={handleSelectProvider}
          />
        )}

        {viewMode === 'accounts' && selectedProvider && (
          <AccountList
            accounts={selectedAccounts}
            providerType={selectedProvider}
            onAddAccount={() => { setEditingAccount(null); setShowAddDialog(true); }}
            onEditAccount={handleEditAccount}
            onDeleteAccount={handleDeleteAccount}
            onValidateAccount={handleValidateAccount}
            onViewDetail={(account) => { setDetailAccount(account); setViewMode('account-detail'); }}
          />
        )}

        {viewMode === 'account-detail' && detailAccount && (
          <div className="space-y-6">
            <div className="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-6">
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-4">
                  <div className={`w-14 h-14 rounded-2xl flex items-center justify-center text-white font-bold text-xl shadow-sm ${
                    detailAccount.status === 'active' ? 'bg-gradient-to-br from-emerald-400 to-emerald-600' :
                    detailAccount.status === 'expired' ? 'bg-gradient-to-br from-amber-400 to-amber-600' :
                    detailAccount.status === 'error' ? 'bg-gradient-to-br from-red-400 to-red-600' :
                    'bg-gradient-to-br from-slate-400 to-slate-600'
                  }`}>
                    {detailAccount.name?.charAt(0).toUpperCase() || '?'}
                  </div>
                  <div>
                    <h2 className="text-xl font-bold text-slate-800">{detailAccount.name}</h2>
                    <div className="flex items-center gap-2 mt-1">
                      <span className={`inline-flex px-2.5 py-0.5 rounded-full text-xs font-semibold ${
                        detailAccount.status === 'active' ? 'bg-emerald-100 text-emerald-700' :
                        detailAccount.status === 'expired' ? 'bg-amber-100 text-amber-700' :
                        detailAccount.status === 'error' ? 'bg-red-100 text-red-700' :
                        'bg-slate-100 text-slate-700'
                      }`}>{detailAccount.status}</span>
                      <span className="text-sm text-slate-500">{detailAccount.providerType}</span>
                    </div>
                  </div>
                </div>
                <div className="flex gap-2">
                  <button onClick={() => handleValidateAccount(detailAccount.id)} className="px-4 py-2 text-sm bg-blue-50 text-blue-600 rounded-xl hover:bg-blue-100 transition-colors font-medium flex items-center gap-1.5">
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                    Validate
                  </button>
                  <button onClick={() => handleEditAccount(detailAccount)} className="px-4 py-2 text-sm bg-slate-50 text-slate-600 rounded-xl hover:bg-slate-100 transition-colors font-medium flex items-center gap-1.5">
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
                    Edit
                  </button>
                  <button onClick={() => handleDeleteAccount(detailAccount.id)} className="px-4 py-2 text-sm bg-red-50 text-red-600 rounded-xl hover:bg-red-100 transition-colors font-medium flex items-center gap-1.5">
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                    Delete
                  </button>
                </div>
              </div>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div className="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-6">
                <h3 className="font-semibold text-slate-800 mb-4 flex items-center gap-2">
                  <svg className="w-5 h-5 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" /></svg>
                  Credentials
                </h3>
                <div className="space-y-3">
                  {Object.entries(detailAccount.credentials).map(([key, value]) => (
                    <div key={key} className="bg-slate-50 rounded-xl p-3.5">
                      <span className="text-xs font-medium text-slate-500 uppercase tracking-wider">{key}</span>
                      <p className="text-sm text-slate-800 font-mono truncate mt-1" title={value}>{value}</p>
                    </div>
                  ))}
                  {Object.keys(detailAccount.credentials).length === 0 && (
                    <p className="text-sm text-slate-400 text-center py-4">No credentials</p>
                  )}
                </div>
                {detailAccount.errorMessage && (
                  <div className="mt-4 bg-red-50 border border-red-200 rounded-xl p-3.5 flex items-start gap-2">
                    <svg className="w-4 h-4 text-red-500 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                    <p className="text-sm text-red-700">{detailAccount.errorMessage}</p>
                  </div>
                )}
              </div>

              <div className="bg-white rounded-2xl border border-slate-200/80 shadow-sm p-6">
                <h3 className="font-semibold text-slate-800 mb-4 flex items-center gap-2">
                  <svg className="w-5 h-5 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" /></svg>
                  Statistics
                </h3>
                <div className="grid grid-cols-2 gap-4">
                  <div className="bg-slate-50 rounded-xl p-3.5">
                    <span className="text-xs text-slate-500">Total Requests</span>
                    <p className="font-semibold text-slate-800 text-lg mt-0.5">{detailAccount.requestCount || 0}</p>
                  </div>
                  <div className="bg-slate-50 rounded-xl p-3.5">
                    <span className="text-xs text-slate-500">Today Used</span>
                    <p className="font-semibold text-slate-800 text-lg mt-0.5">{detailAccount.todayUsed || 0}{detailAccount.dailyLimit ? ` / ${detailAccount.dailyLimit}` : ''}</p>
                  </div>
                  <div className="bg-slate-50 rounded-xl p-3.5">
                    <span className="text-xs text-slate-500">Last Validated</span>
                    <p className="font-semibold text-slate-800 text-sm mt-0.5">{detailAccount.lastValidated ? new Date(detailAccount.lastValidated).toLocaleString() : '-'}</p>
                  </div>
                  <div className="bg-slate-50 rounded-xl p-3.5">
                    <span className="text-xs text-slate-500">Created</span>
                    <p className="font-semibold text-slate-800 text-sm mt-0.5">{detailAccount.createdAt ? new Date(detailAccount.createdAt).toLocaleString() : '-'}</p>
                  </div>
                </div>
                {detailAccount.accountInfo && (
                  <div className="mt-4 pt-4 border-t border-slate-100">
                    <h4 className="text-xs font-medium text-slate-500 mb-3 uppercase tracking-wider">Account Info</h4>
                    <div className="space-y-2">
                      {detailAccount.accountInfo.name && (
                        <div className="flex items-center gap-2 text-sm">
                          <svg className="w-4 h-4 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                          <span className="text-slate-700">{detailAccount.accountInfo.name}</span>
                        </div>
                      )}
                      {detailAccount.accountInfo.email && (
                        <div className="flex items-center gap-2 text-sm">
                          <svg className="w-4 h-4 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 8l7.89 5.26a2 2 0 002.22 0L21 8M5 19h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" /></svg>
                          <span className="text-slate-700">{detailAccount.accountInfo.email}</span>
                        </div>
                      )}
                      {detailAccount.accountInfo.userId && (
                        <div className="flex items-center gap-2 text-sm">
                          <svg className="w-4 h-4 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 6H5a2 2 0 00-2 2v9a2 2 0 002 2h14a2 2 0 002-2V8a2 2 0 00-2-2h-5m-4 0V5a2 2 0 114 0v1m-4 0a2 2 0 104 0" /></svg>
                          <span className="text-slate-700 font-mono text-xs">{detailAccount.accountInfo.userId}</span>
                        </div>
                      )}
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        )}
      </main>

      <AddAccountDialog
        key={editingAccount?.id || 'new-account'}
        open={showAddDialog}
        onOpenChange={(open) => { setShowAddDialog(open); if (!open) setEditingAccount(null); }}
        provider={selectedProviderInfo || null}
        onAddAccount={handleAddAccount}
        editingAccount={editingAccount}
      />
    </div>
  );
}
