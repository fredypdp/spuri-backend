// src/lib/api/client.ts

import { getCookie, setCookie, removeCookie } from '@/lib/utils/cookies';

// ✅ URL base com protocolo garantido
const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || process.env.API_URL;

export const getApiBaseUrl = () => {
  const url = API_BASE_URL;
  
  if (url && !url.startsWith('http://') && !url.startsWith('https://')) {
    return `https://${url}`;
  }
  
  return url;
};

export interface FetchOptions extends RequestInit {
  token?: string;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public data?: any
  ) {
    // ✅ Extrai a melhor mensagem disponível automaticamente
    const message = extractErrorMessage(data, statusText);
    super(message);
    this.name = 'ApiError';
  }
}

// ✅ EXPORTADO: Erro específico da API Spuri
export class SpuriApiError extends ApiError {
  constructor(
    status: number,
    statusText: string,
    data?: any
  ) {
    super(status, statusText, data);
    this.name = 'SpuriApiError';
  }
}

/**
 * ✅ Extrai a melhor mensagem de erro disponível
 * Prioridade: data.message > data.error > statusText
 */
function extractErrorMessage(data: any, statusText: string): string {
  if (data?.message) return data.message;
  if (data?.error) return data.error;
  return statusText || 'Erro desconhecido';
}

/**
 * Cliente HTTP base para fazer requisições à API
 */
async function fetchApi<T>(
  endpoint: string,
  options: FetchOptions = {}
): Promise<T> {
  const { token, ...fetchOptions } = options;

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(fetchOptions.headers as Record<string, string>),
  };

  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  const baseUrl = getApiBaseUrl();
  
  if (!baseUrl) {
    throw new Error('API_URL não está configurada. Defina NEXT_PUBLIC_API_URL no arquivo .env');
  }

  const url = `${baseUrl}${endpoint}`;

  const response = await fetch(url, {
    ...fetchOptions,
    headers,
  });

  // ✅ Tratamento de erro ANTES do parse
  if (!response.ok) {
    let errorData: any = null;
    
    try {
      errorData = await response.json();
    } catch (parseError) {
      errorData = { error: response.statusText };
    }
    
    // ✅ Log detalhado do erro (apenas em dev)
    if (process.env.NODE_ENV === 'development') {
      console.error('❌ API Request Failed:', {
        url,
        status: response.status,
        statusText: response.statusText,
        errorData
      });
    }
    
    // ✅ SpuriApiError já extrai a mensagem no construtor
    throw new SpuriApiError(response.status, response.statusText, errorData);
  }

  // Se a resposta for 204 (No Content), retorna objeto vazio
  if (response.status === 204) {
    return {} as T;
  }

  return response.json();
}

// Métodos HTTP convenientes
export const api = {
  get: <T>(endpoint: string, options?: FetchOptions) =>
    fetchApi<T>(endpoint, { ...options, method: 'GET' }),

  post: <T>(endpoint: string, data?: any, options?: FetchOptions) =>
    fetchApi<T>(endpoint, {
      ...options,
      method: 'POST',
      body: data ? JSON.stringify(data) : undefined,
    }),

  put: <T>(endpoint: string, data?: any, options?: FetchOptions) =>
    fetchApi<T>(endpoint, {
      ...options,
      method: 'PUT',
      body: data ? JSON.stringify(data) : undefined,
    }),

  delete: <T>(endpoint: string, options?: FetchOptions) =>
    fetchApi<T>(endpoint, { ...options, method: 'DELETE' }),

  patch: <T>(endpoint: string, data?: any, options?: FetchOptions) =>
    fetchApi<T>(endpoint, {
      ...options,
      method: 'PATCH',
      body: data ? JSON.stringify(data) : undefined,
    }),
};

// Storage helpers para token (usando cookies)
export const tokenStorage = {
  get: () => {
    if (typeof window === 'undefined') return null;
    return getCookie('auth_token');
  },

  set: (token: string) => {
    if (typeof window === 'undefined') return;
    setCookie('auth_token', token, 1); // Expira em 1 dia (24h)
  },

  remove: () => {
    if (typeof window === 'undefined') return;
    removeCookie('auth_token');
    removeCookie('user_type');
    removeCookie('user');
  },

  getWithType: () => {
    if (typeof window === 'undefined') return { token: null, type: null };
    const token = getCookie('auth_token');
    const type = getCookie('user_type');
    return { token, type };
  },

  setWithType: (token: string, type: string) => {
    if (typeof window === 'undefined') return;
    setCookie('auth_token', token, 1);
    setCookie('user_type', type, 1);
  },
};
