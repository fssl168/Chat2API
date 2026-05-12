export type ProviderType = 'deepseek' | 'glm' | 'kimi' | 'minimax' | 'qwen' | 'qwen-ai' | 'zai' | 'perplexity' | 'mimo';

export interface CredentialField {
  name: string;
  label: string;
  type: 'text' | 'password' | 'textarea';
  required: boolean;
  placeholder?: string;
  helpText?: string;
}

export interface Provider {
  id: string;
  providerType: ProviderType;
  label: string;
  description: string;
  helpUrl?: string;
  loginURL: string;
  supportedModels?: string[];
  authType: string;
  credentialFields: CredentialField[];
  enabled: boolean;
}

export interface Account {
  id: string;
  providerId: string;
  providerType: ProviderType;
  name: string;
  credentials: Record<string, string>;
  status: 'active' | 'expired' | 'error' | 'inactive';
  accountInfo?: {
    userId?: string;
    email?: string;
    name?: string;
  };
  dailyLimit?: number;
  todayUsed?: number;
  requestCount?: number;
  errorMessage?: string;
  lastUsed?: string;
  lastValidated?: string;
  createdAt?: string;
}

export interface ApiResponse<T> {
  success: boolean;
  data?: T;
  error?: string;
  message?: string;
}

export interface BrowserTask {
  id: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  progress?: string;
  result?: {
    credentials?: Record<string, string>;
    accountInfo?: {
      userId?: string;
      email?: string;
      name?: string;
    };
    error?: string;
  };
}

export interface TokenValidationResult {
  valid: boolean;
  error?: string;
  tokenType?: string;
  accountInfo?: {
    userId?: string;
    email?: string;
    name?: string;
  };
}
