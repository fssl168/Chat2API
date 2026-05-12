import type { Provider, ProviderType, Account } from '../api/types';

const providerIcons: Record<string, string> = {
  deepseek: '🔍',
  glm: '🧠',
  kimi: '🌙',
  minimax: '⚡',
  qwen: '☁️',
  'qwen-ai': '🌐',
  zai: '🤖',
  perplexity: '🔮',
  mimo: '📱',
};

const providerGradients: Record<string, string> = {
  deepseek: 'from-blue-500 to-blue-700',
  glm: 'from-emerald-500 to-emerald-700',
  kimi: 'from-purple-500 to-purple-700',
  minimax: 'from-amber-500 to-amber-700',
  qwen: 'from-orange-500 to-orange-700',
  'qwen-ai': 'from-teal-500 to-teal-700',
  zai: 'from-stone-500 to-stone-700',
  perplexity: 'from-slate-500 to-slate-700',
  mimo: 'from-indigo-500 to-indigo-700',
};

const providerLightBg: Record<string, string> = {
  deepseek: 'bg-blue-50',
  glm: 'bg-emerald-50',
  kimi: 'bg-purple-50',
  minimax: 'bg-amber-50',
  qwen: 'bg-orange-50',
  'qwen-ai': 'bg-teal-50',
  zai: 'bg-stone-50',
  perplexity: 'bg-slate-50',
  mimo: 'bg-indigo-50',
};

interface ProviderListProps {
  providers: Provider[];
  accounts: Record<string, Account[]>;
  onSelect: (providerType: ProviderType) => void;
}

function getAuthLabel(authType: string): string {
  const labels: Record<string, string> = {
    userToken: 'User Token',
    jwt: 'JWT',
    refresh_token: 'Refresh Token',
    cookie: 'Cookie',
    tongyi_sso_ticket: 'SSO Ticket',
    token: 'Token',
  };
  return labels[authType] || authType;
}

export function ProviderList({ providers, accounts, onSelect }: ProviderListProps) {
  return (
    <div>
      <div className="mb-6">
        <h2 className="text-2xl font-bold text-slate-800">AI Providers</h2>
        <p className="text-slate-500 mt-1">Select a provider to manage accounts and credentials</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
        {providers.map(provider => {
          const type = provider.providerType;
          const providerAccounts = accounts[type] || [];
          const activeCount = providerAccounts.filter(a => a.status === 'active').length;
          const totalCount = providerAccounts.length;
          const gradient = providerGradients[type] || 'from-gray-500 to-gray-700';
          const lightBg = providerLightBg[type] || 'bg-gray-50';

          return (
            <div
              key={type}
              onClick={() => onSelect(type as ProviderType)}
              className="bg-white rounded-2xl border border-slate-200/80 shadow-sm hover:shadow-lg hover:border-slate-300/80 transition-all duration-300 cursor-pointer group overflow-hidden"
            >
              <div className={`h-1.5 bg-gradient-to-r ${gradient}`} />

              <div className="p-5">
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3">
                    <div className={`w-11 h-11 rounded-xl ${lightBg} flex items-center justify-center text-xl shadow-sm`}>
                      {providerIcons[type] || '📦'}
                    </div>
                    <div>
                      <h3 className="font-semibold text-slate-800 group-hover:text-indigo-600 transition-colors text-base">
                        {provider.label}
                      </h3>
                      <p className="text-xs text-slate-400 mt-0.5 line-clamp-1">{provider.description}</p>
                    </div>
                  </div>
                  <div className="flex items-center gap-1.5 shrink-0">
                    <div className={`w-2 h-2 rounded-full ${activeCount > 0 ? 'bg-emerald-500' : totalCount > 0 ? 'bg-amber-400' : 'bg-slate-300'}`} />
                    <span className="text-xs text-slate-400">
                      {activeCount > 0 ? `${activeCount} active` : totalCount > 0 ? 'Inactive' : 'No accounts'}
                    </span>
                  </div>
                </div>

                <div className="grid grid-cols-3 gap-3 text-sm mb-4 py-3 px-4 bg-slate-50/80 rounded-xl">
                  <div className="flex flex-col">
                    <span className="text-[10px] text-slate-400 uppercase tracking-wider font-medium">Accounts</span>
                    <span className="font-semibold text-slate-700">{activeCount}<span className="text-slate-400 font-normal">/{totalCount}</span></span>
                  </div>
                  <div className="flex flex-col">
                    <span className="text-[10px] text-slate-400 uppercase tracking-wider font-medium">Models</span>
                    <span className="font-semibold text-slate-700">{provider.supportedModels?.length || 0}</span>
                  </div>
                  <div className="flex flex-col">
                    <span className="text-[10px] text-slate-400 uppercase tracking-wider font-medium">Auth</span>
                    <span className="font-semibold text-slate-700 text-xs truncate">{getAuthLabel(provider.authType)}</span>
                  </div>
                </div>

                {provider.supportedModels && provider.supportedModels.length > 0 && (
                  <div className="mb-4 flex flex-wrap gap-1.5">
                    {provider.supportedModels.slice(0, 4).map(model => (
                      <span key={model} className="px-2 py-0.5 bg-slate-100 text-slate-500 rounded-md text-[10px] font-medium">
                        {model}
                      </span>
                    ))}
                    {provider.supportedModels.length > 4 && (
                      <span className="px-2 py-0.5 bg-slate-100 text-slate-400 rounded-md text-[10px]">
                        +{provider.supportedModels.length - 4}
                      </span>
                    )}
                  </div>
                )}

                <button
                  onClick={e => { e.stopPropagation(); onSelect(type as ProviderType); }}
                  className={`w-full py-2.5 px-4 rounded-xl font-medium transition-all duration-200 text-sm flex items-center justify-center gap-2 ${
                    totalCount === 0
                      ? `bg-gradient-to-r ${gradient} text-white hover:opacity-90 shadow-sm`
                      : `bg-slate-50 border border-slate-200 text-slate-600 hover:bg-slate-100 hover:border-slate-300`
                  }`}
                >
                  {totalCount === 0 ? (
                    <>
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" /></svg>
                      Add Account
                    </>
                  ) : (
                    <>
                      <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" /></svg>
                      Manage ({activeCount}/{totalCount})
                    </>
                  )}
                </button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
