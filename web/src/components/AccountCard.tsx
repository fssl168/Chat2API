import { useState } from 'react';
import type { Account } from '@/api/types';
import { validateAccount as apiValidateAccount } from '@/api/client';

interface AccountListProps {
  accounts: Account[];
  providerType: string;
  onAddAccount?: () => void;
  onEditAccount?: (account: Account) => void;
  onDeleteAccount?: (id: string) => void;
  onValidateAccount?: (id: string) => void;
  onViewDetail?: (account: Account) => void;
}

const statusConfig: Record<string, { label: string; color: string; bgColor: string; dotColor: string; icon: string }> = {
  active: { label: 'Active', color: 'text-emerald-700', bgColor: 'bg-emerald-50 border-emerald-200', dotColor: 'bg-emerald-500', icon: '✓' },
  inactive: { label: 'Inactive', color: 'text-slate-600', bgColor: 'bg-slate-50 border-slate-200', dotColor: 'bg-slate-400', icon: '○' },
  expired: { label: 'Expired', color: 'text-amber-700', bgColor: 'bg-amber-50 border-amber-200', dotColor: 'bg-amber-500', icon: '⏰' },
  error: { label: 'Error', color: 'text-red-700', bgColor: 'bg-red-50 border-red-200', dotColor: 'bg-red-500', icon: '✕' },
};

export function AccountList({ accounts, providerType, onAddAccount, onEditAccount, onDeleteAccount, onValidateAccount, onViewDetail }: AccountListProps) {
  const [validatingIds, setValidatingIds] = useState<Set<string>>(new Set());
  const [menuOpenId, setMenuOpenId] = useState<string | null>(null);
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);

  const activeCount = accounts.filter(a => a.status === 'active').length;

  const handleValidate = async (id: string) => {
    setValidatingIds(prev => new Set(prev).add(id));
    try {
      const result = await apiValidateAccount(providerType, id);
      if (result.success && result.data?.account) {
        onValidateAccount?.(id);
      }
    } catch (err) {
      console.error('Validate failed:', err);
    }
    setTimeout(() => {
      setValidatingIds(prev => { const n = new Set(prev); n.delete(id); return n; });
    }, 2000);
  };

  if (accounts.length === 0) {
    return (
      <div className="text-center py-20">
        <div className="w-20 h-20 bg-slate-100 rounded-2xl flex items-center justify-center mx-auto mb-5">
          <svg className="w-10 h-10 text-slate-300" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z" />
          </svg>
        </div>
        <h3 className="text-lg font-semibold text-slate-600 mb-2">No accounts yet</h3>
        <p className="text-sm text-slate-400 mb-6">Add your first {providerType} account to get started</p>
        <button onClick={onAddAccount} className="px-6 py-2.5 bg-indigo-600 text-white rounded-xl hover:bg-indigo-700 transition-colors font-medium text-sm shadow-sm">
          Add Account
        </button>
      </div>
    );
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-5">
        <div className="flex items-center gap-3">
          <span className="text-sm text-slate-500">{accounts.length} account(s)</span>
          <span className="text-slate-300">•</span>
          <span className="text-sm text-emerald-600 font-medium">{activeCount} active</span>
        </div>
      </div>

      <div className="space-y-3">
        {accounts.map(account => {
          const status = statusConfig[account.status] || statusConfig.inactive;
          const isValidating = validatingIds.has(account.id);

          return (
            <div key={account.id} className="bg-white rounded-xl border border-slate-200/80 shadow-sm hover:shadow-md transition-all duration-200 overflow-hidden group">
              <div className="p-4 flex items-center justify-between">
                <div className="flex items-center gap-3 flex-1 min-w-0 cursor-pointer" onClick={() => onViewDetail?.(account)}>
                  <div className={`w-10 h-10 rounded-xl flex items-center justify-center text-white font-semibold text-sm shrink-0 ${status.dotColor} shadow-sm`}>
                    {account.name?.charAt(0).toUpperCase() || '?'}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-slate-800 truncate">{account.name}</span>
                      <span className={`inline-flex px-2 py-0.5 rounded-full text-[10px] font-semibold ${status.bgColor} ${status.color} border`}>
                        {status.icon} {status.label}
                      </span>
                    </div>
                    <div className="flex items-center gap-3 mt-1 text-xs text-slate-400">
                      {account.accountInfo?.email && <span>{account.accountInfo.email}</span>}
                      {account.accountInfo?.name && account.accountInfo.name !== account.name && <span>{account.accountInfo.name}</span>}
                      {account.lastValidated && (
                        <span>Validated: {new Date(account.lastValidated).toLocaleDateString()}</span>
                      )}
                    </div>
                    {account.errorMessage && <p className="text-xs text-red-500 mt-1 truncate">{account.errorMessage}</p>}
                  </div>
                </div>

                <div className="flex items-center gap-1 shrink-0 ml-3">
                  <button
                    onClick={() => handleValidate(account.id)}
                    disabled={isValidating}
                    className="p-2 hover:bg-blue-50 rounded-lg transition-colors text-blue-500 disabled:opacity-50"
                    title="Validate"
                  >
                    {isValidating ? (
                      <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
                      </svg>
                    ) : (
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                    )}
                  </button>

                  <div className="relative">
                    <button
                      onClick={() => setMenuOpenId(menuOpenId === account.id ? null : account.id)}
                      className="p-2 hover:bg-slate-50 rounded-lg transition-colors"
                    >
                      <svg className="w-4 h-4 text-slate-400 group-hover:text-slate-600" fill="currentColor" viewBox="0 0 20 20">
                        <path d="M10 6a2 2 0 110-4 2 2 0 010 4zM10 12a2 2 0 110-4 2 2 0 010 4zM10 18a2 2 0 110-4 2 2 0 010 4z" />
                      </svg>
                    </button>
                    {menuOpenId === account.id && (
                      <>
                        <div className="fixed inset-0 z-10" onClick={() => setMenuOpenId(null)} />
                        <div className="absolute right-0 mt-1 w-48 bg-white rounded-xl shadow-lg border border-slate-200 py-1 z-20">
                          <button onClick={() => { onViewDetail?.(account); setMenuOpenId(null); }} className="w-full px-4 py-2.5 text-left text-sm text-slate-700 hover:bg-slate-50 flex items-center gap-3 transition-colors">
                            <svg className="w-4 h-4 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2" /></svg>
                            View Details
                          </button>
                          <button onClick={() => { onEditAccount?.(account); setMenuOpenId(null); }} className="w-full px-4 py-2.5 text-left text-sm text-slate-700 hover:bg-slate-50 flex items-center gap-3 transition-colors">
                            <svg className="w-4 h-4 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" /></svg>
                            Edit Account
                          </button>
                          <hr className="my-1 border-slate-100" />
                          <button onClick={() => { setDeleteConfirmId(account.id); setMenuOpenId(null); }} className="w-full px-4 py-2.5 text-left text-sm text-red-600 hover:bg-red-50 flex items-center gap-3 transition-colors">
                            <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                            Delete Account
                          </button>
                        </div>
                      </>
                    )}
                  </div>
                </div>
              </div>
            </div>
          );
        })}
      </div>

      {deleteConfirmId && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 backdrop-blur-sm" onClick={() => setDeleteConfirmId(null)}>
          <div className="bg-white rounded-2xl shadow-2xl p-6 max-w-sm w-full mx-4" onClick={e => e.stopPropagation()}>
            <div className="w-12 h-12 bg-red-100 rounded-xl flex items-center justify-center mx-auto mb-4">
              <svg className="w-6 h-6 text-red-600" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" /></svg>
            </div>
            <h3 className="text-lg font-semibold text-slate-800 text-center mb-2">Delete Account?</h3>
            <p className="text-sm text-slate-500 text-center mb-6">This action cannot be undone. The account will be permanently removed.</p>
            <div className="flex gap-3 justify-center">
              <button onClick={() => setDeleteConfirmId(null)} className="px-5 py-2.5 text-sm text-slate-600 hover:bg-slate-50 rounded-xl transition-colors font-medium">Cancel</button>
              <button onClick={() => { onDeleteAccount?.(deleteConfirmId); setDeleteConfirmId(null); }} className="px-5 py-2.5 text-sm bg-red-600 text-white rounded-xl hover:bg-red-700 transition-colors font-medium shadow-sm">Delete</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
