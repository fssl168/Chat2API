import { useState, useEffect } from 'react';
import type { Provider, CredentialField, Account } from '@/api/types';
import { loginWithBrowser, pollBrowserTask, validateToken } from '@/api/client';

interface AddAccountDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: Provider | null;
  onAddAccount: (account: Account) => void;
  editingAccount?: Account | null;
}

function mapOAuthCredentials(providerId: string | undefined, credentials: Record<string, string>): Record<string, string> {
  if (!providerId) return credentials;

  if (providerId === 'kimi') {
    const possibleKeys = ['token', 'Authorization', 'kimi-auth'];
    let jwtVal = '';
    let fallbackVal = '';
    for (const key of possibleKeys) {
      const val = credentials[key];
      if (val) {
        if (val.startsWith('eyJ')) { jwtVal = val; break; }
        else if (!fallbackVal) { fallbackVal = val; }
      }
    }
    const finalValue = jwtVal || fallbackVal;
    if (finalValue) return { token: finalValue };
  }

  const keyMap: Record<string, { src: string; dst: string }> = {
    glm: { src: 'chatglm_refresh_token', dst: 'refresh_token' },
    deepseek: { src: 'token', dst: 'token' },
    minimax: { src: 'token', dst: 'token' },
    qwen: { src: 'tongyi_sso_ticket', dst: 'ticket' },
    'qwen-ai': { src: 'token', dst: 'token' },
    zai: { src: 'token', dst: 'token' },
    perplexity: { src: '__Secure-next-auth.session-token', dst: 'sessionToken' },
  };

  const mapping = keyMap[providerId];
  if (mapping && credentials[mapping.src]) {
    return { [mapping.dst]: credentials[mapping.src] };
  }

  if (providerId === 'perplexity' && credentials['next-auth.session-token']) {
    return { sessionToken: credentials['next-auth.session-token'] };
  }

  if (providerId === 'mimo') {
    const result: Record<string, string> = {};
    if (credentials['serviceToken']) result['service_token'] = credentials['serviceToken'];
    if (credentials['userId']) result['user_id'] = credentials['userId'];
    if (credentials['xiaomichatbot_ph']) result['ph_token'] = credentials['xiaomichatbot_ph'];
    return result;
  }

  return credentials;
}

export function AddAccountDialog({ open, onOpenChange, provider, onAddAccount, editingAccount }: AddAccountDialogProps) {
  const [tab, setTab] = useState<'manual' | 'browser'>('manual');
  const [name, setName] = useState('');
  const [dailyLimit, setDailyLimit] = useState('');
  const [credentialValues, setCredentialValues] = useState<Record<string, string>>({});
  const [showPasswords, setShowPasswords] = useState<Record<string, boolean>>({});
  const [isValidating, setIsValidating] = useState(false);
  const [validationResult, setValidationResult] = useState<{ valid: boolean; info?: string; error?: string } | null>(null);
  const [browserStatus, setBrowserStatus] = useState<string>('');
  const [isBrowserLoggingIn, setIsBrowserLoggingIn] = useState(false);

  const providerId = provider?.providerType;
  const fields: CredentialField[] = provider?.credentialFields || [];
  const isEditing = !!editingAccount;

  useEffect(() => {
    if (open && editingAccount) {
      setName(editingAccount.name || '');
      setDailyLimit(editingAccount.dailyLimit?.toString() || '');
      setCredentialValues({ ...editingAccount.credentials });
      setTab('manual');
      setValidationResult(null);
    } else if (open) {
      setName('');
      setDailyLimit('');
      setCredentialValues({});
      setTab('manual');
      setValidationResult(null);
      setBrowserStatus('');
      setIsBrowserLoggingIn(false);
    }
  }, [open, editingAccount]);

  const handleValidate = async () => {
    if (!providerId) return;
    setIsValidating(true);
    setValidationResult(null);
    try {
      const result = await validateToken({ providerId, providerType: providerId, credentials: credentialValues });
      if (result.data?.valid) {
        const info = result.data.accountInfo;
        setValidationResult({ valid: true, info: [info?.name, info?.email, info?.userId].filter(Boolean).join(' • ') || 'Valid token' });
        if (!name && info?.name) setName(info.name);
      } else {
        setValidationResult({ valid: false, error: result.data?.error || result.error || 'Validation failed' });
      }
    } catch (err) {
      setValidationResult({ valid: false, error: err instanceof Error ? err.message : 'Validation error' });
    }
    setIsValidating(false);
  };

  const handleBrowserLogin = async () => {
    if (!providerId) return;
    setIsBrowserLoggingIn(true);
    setBrowserStatus('Opening browser...');
    setValidationResult(null);
    try {
      const startResult = await loginWithBrowser({ providerId, providerType: providerId, timeout: 300 });
      if (!startResult.success || !startResult.data?.taskId) {
        setBrowserStatus(`Failed: ${startResult.error || 'No task ID'}`);
        setIsBrowserLoggingIn(false);
        return;
      }
      setBrowserStatus('Waiting for login...');

      const taskResult = await pollBrowserTask(startResult.data.taskId, (task) => {
        setBrowserStatus(task.status === 'running' ? 'Waiting for login...' : task.status);
      });

      if (taskResult.status === 'completed' && taskResult.result?.credentials) {
        const mapped = mapOAuthCredentials(providerId, taskResult.result.credentials);
        setCredentialValues(prev => ({ ...prev, ...mapped }));
        setBrowserStatus('Login successful! Credentials captured.');
        if (taskResult.result.accountInfo?.name && !name) {
          setName(taskResult.result.accountInfo.name);
        }
        setValidationResult({ valid: true, info: taskResult.result.accountInfo?.name || 'Token captured' });
      } else {
        setBrowserStatus(taskResult.result?.error || 'Login failed');
        setValidationResult({ valid: false, error: taskResult.result?.error || 'Login failed' });
      }
    } catch (err) {
      setBrowserStatus(`Error: ${err instanceof Error ? err.message : 'Unknown'}`);
    }
    setIsBrowserLoggingIn(false);
  };

  const handleSubmit = () => {
    if (!providerId || !name) return;
    const account: Account = {
      id: editingAccount?.id || `${providerId}-${Date.now()}`,
      providerId: providerId,
      providerType: providerId as any,
      name,
      credentials: { ...credentialValues },
      status: validationResult?.valid ? 'active' : 'inactive',
      accountInfo: validationResult?.valid ? { name: validationResult.info } : undefined,
      dailyLimit: dailyLimit ? parseInt(dailyLimit) : undefined,
      createdAt: editingAccount?.createdAt || new Date().toISOString(),
    };
    onAddAccount(account);
  };

  if (!open || !provider) return null;

  const canSubmit = name && fields.every(f => !f.required || credentialValues[f.name]);

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4 backdrop-blur-sm" onClick={() => onOpenChange(false)}>
      <div className="bg-white rounded-2xl shadow-2xl max-w-lg w-full max-h-[90vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <div className="px-6 py-5 border-b border-slate-100">
          <div className="flex items-center justify-between">
            <div>
              <h2 className="text-lg font-semibold text-slate-800">{isEditing ? 'Edit Account' : 'Add Account'}</h2>
              <p className="text-sm text-slate-500 mt-0.5">{provider.label}</p>
            </div>
            <button onClick={() => onOpenChange(false)} className="p-2 hover:bg-slate-100 rounded-lg transition-colors">
              <svg className="w-5 h-5 text-slate-400" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
          </div>
        </div>

        <div className="px-6 py-5 space-y-5">
          <div>
            <label className="block text-sm font-medium text-slate-700 mb-1.5">Account Name <span className="text-red-500">*</span></label>
            <input
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="e.g. My DeepSeek Account"
              className="w-full px-3.5 py-2.5 border border-slate-200 rounded-xl text-sm focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none transition-all"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-slate-700 mb-1.5">Daily Limit (optional)</label>
            <input
              type="number"
              value={dailyLimit}
              onChange={e => setDailyLimit(e.target.value)}
              placeholder="Unlimited"
              className="w-full px-3.5 py-2.5 border border-slate-200 rounded-xl text-sm focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none transition-all"
            />
          </div>

          {!isEditing && (
            <div className="flex bg-slate-100 rounded-xl p-1">
              <button
                onClick={() => setTab('manual')}
                className={`flex-1 py-2.5 text-sm font-medium rounded-lg transition-all duration-200 ${
                  tab === 'manual' ? 'bg-white text-slate-800 shadow-sm' : 'text-slate-500 hover:text-slate-700'
                }`}
              >
                Manual Input
              </button>
              <button
                onClick={() => setTab('browser')}
                className={`flex-1 py-2.5 text-sm font-medium rounded-lg transition-all duration-200 ${
                  tab === 'browser' ? 'bg-white text-slate-800 shadow-sm' : 'text-slate-500 hover:text-slate-700'
                }`}
              >
                Browser Login
              </button>
            </div>
          )}

          {tab === 'manual' && (
            <div className="space-y-4">
              {fields.map(field => (
                <div key={field.name}>
                  <label className="block text-sm font-medium text-slate-700 mb-1.5">
                    {field.label} {field.required && <span className="text-red-500">*</span>}
                  </label>
                  {field.type === 'textarea' ? (
                    <textarea
                      value={credentialValues[field.name] || ''}
                      onChange={e => setCredentialValues(prev => ({ ...prev, [field.name]: e.target.value }))}
                      placeholder={field.placeholder}
                      rows={3}
                      className="w-full px-3.5 py-2.5 border border-slate-200 rounded-xl text-sm focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none transition-all resize-none font-mono"
                    />
                  ) : (
                    <div className="relative">
                      <input
                        type={field.type === 'password' && !showPasswords[field.name] ? 'password' : 'text'}
                        value={credentialValues[field.name] || ''}
                        onChange={e => setCredentialValues(prev => ({ ...prev, [field.name]: e.target.value }))}
                        placeholder={field.placeholder}
                        className="w-full px-3.5 py-2.5 border border-slate-200 rounded-xl text-sm focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 outline-none transition-all pr-20 font-mono"
                      />
                      <div className="absolute right-1.5 top-1.5 flex gap-0.5">
                        {field.type === 'password' && (
                          <button
                            onClick={() => setShowPasswords(prev => ({ ...prev, [field.name]: !prev[field.name] }))}
                            className="px-2 py-1.5 text-xs text-slate-400 hover:text-slate-600 rounded-lg hover:bg-slate-50 transition-colors"
                          >
                            {showPasswords[field.name] ? 'Hide' : 'Show'}
                          </button>
                        )}
                        <button
                          onClick={() => { navigator.clipboard.writeText(credentialValues[field.name] || ''); }}
                          className="px-2 py-1.5 text-xs text-slate-400 hover:text-slate-600 rounded-lg hover:bg-slate-50 transition-colors"
                        >
                          Copy
                        </button>
                      </div>
                    </div>
                  )}
                  {field.helpText && (
                    <p className="text-xs text-slate-400 mt-1.5 flex items-start gap-1.5">
                      <svg className="w-3.5 h-3.5 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                      {field.helpText}
                    </p>
                  )}
                </div>
              ))}
            </div>
          )}

          {tab === 'browser' && !isEditing && (
            <div className="space-y-4">
              <div className="bg-indigo-50 border border-indigo-100 rounded-xl p-4">
                <div className="flex items-start gap-3">
                  <svg className="w-5 h-5 text-indigo-500 shrink-0 mt-0.5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                  <div>
                    <p className="text-sm text-indigo-700 font-medium">Browser Auto-Login</p>
                    <p className="text-xs text-indigo-600/70 mt-1">Click the button below to open a browser window. Log in with your {provider.label} account, and the token will be automatically captured.</p>
                  </div>
                </div>
              </div>
              <button
                onClick={handleBrowserLogin}
                disabled={isBrowserLoggingIn}
                className="w-full py-3 bg-gradient-to-r from-indigo-600 to-indigo-700 text-white rounded-xl hover:from-indigo-700 hover:to-indigo-800 transition-all font-medium text-sm disabled:opacity-50 flex items-center justify-center gap-2 shadow-sm"
              >
                {isBrowserLoggingIn ? (
                  <>
                    <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" /><path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" /></svg>
                    {browserStatus}
                  </>
                ) : (
                  <>
                    <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 01-9 9m9-9a9 9 0 00-9-9m9 9H3m9 9a9 9 0 01-9-9m9 9c1.657 0 3-4.03 3-9s-1.343-9-3-9m0 18c-1.657 0-3-4.03-3-9s1.343-9 3-9m-9 9a9 9 0 019-9" /></svg>
                    Open Browser Login
                  </>
                )}
              </button>
              {browserStatus && !isBrowserLoggingIn && (
                <div className={`text-sm p-3.5 rounded-xl flex items-center gap-2 ${
                  browserStatus.includes('successful') ? 'bg-emerald-50 text-emerald-700 border border-emerald-200' :
                  browserStatus.includes('Failed') || browserStatus.includes('Error') ? 'bg-red-50 text-red-700 border border-red-200' :
                  'bg-slate-50 text-slate-600 border border-slate-200'
                }`}>
                  {browserStatus.includes('successful') ? (
                    <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                  ) : browserStatus.includes('Failed') || browserStatus.includes('Error') ? (
                    <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
                  ) : null}
                  {browserStatus}
                </div>
              )}
            </div>
          )}

          {validationResult && (
            <div className={`p-3.5 rounded-xl text-sm flex items-center gap-2 ${
              validationResult.valid ? 'bg-emerald-50 text-emerald-700 border border-emerald-200' : 'bg-red-50 text-red-700 border border-red-200'
            }`}>
              {validationResult.valid ? (
                <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
              ) : (
                <svg className="w-4 h-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg>
              )}
              {validationResult.valid ? validationResult.info : validationResult.error}
            </div>
          )}

          <div className="flex gap-3 pt-2">
            <button
              onClick={handleValidate}
              disabled={isValidating || !Object.values(credentialValues).some(v => v)}
              className="px-4 py-2.5 text-sm bg-blue-50 text-blue-600 rounded-xl hover:bg-blue-100 transition-colors font-medium disabled:opacity-50 flex items-center gap-2"
            >
              {isValidating && <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24"><circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" /><path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" /></svg>}
              Validate
            </button>
            <div className="flex-1" />
            <button onClick={() => onOpenChange(false)} className="px-5 py-2.5 text-sm text-slate-600 hover:bg-slate-50 rounded-xl transition-colors font-medium">Cancel</button>
            <button
              onClick={handleSubmit}
              disabled={!canSubmit}
              className="px-5 py-2.5 text-sm bg-gradient-to-r from-indigo-600 to-indigo-700 text-white rounded-xl hover:from-indigo-700 hover:to-indigo-800 transition-all font-medium disabled:opacity-50 shadow-sm"
            >
              {isEditing ? 'Save Changes' : 'Add Account'}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
