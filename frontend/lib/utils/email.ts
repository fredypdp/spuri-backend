// src/lib/utils/email.ts
import { UserType } from "@/types/api";
import { tokenStorage } from '@/lib/api';

export async function VerificarEmailComFrontend(identificador: string, tipo: UserType) {
  const jwt = tokenStorage.get();
  if (!jwt) throw new Error('Usuário não autenticado');

  const response = await fetch('/api/verificar-email', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${jwt}`,   // ← enviar JWT
    },
    body: JSON.stringify({ identificador, tipo }),
  });

  const data = await response.json();
  if (!response.ok) throw new Error(data.message || data.error || 'Erro ao enviar email');
  return data;
}

/**
 * ✅ SIMPLIFICADO: Tudo acontece no API route
 */
export async function RecuperarSenhaComFrontend(identificador: string, tipo: UserType) {
  // Validação
  if (!identificador || identificador.trim() === '') {
    throw new Error('Identificador é obrigatório');
  }

  if (!tipo || !['estudante', 'academia', 'admin'].includes(tipo)) {
    throw new Error('Tipo de usuário inválido');
  }

  const response = await fetch('/api/recuperar-senha', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identificador, tipo }),
  });

  const data = await response.json();

  if (!response.ok) {
    // ✅ Prioridade: message > error
    const errorMsg = data.message || data.error || 'Erro ao recuperar senha';
    throw new Error(errorMsg);
  }
  
  return data;
}

/**
 * Verifica email usando token da URL
 */
export async function VerificarEmailComToken(token: string) {
  if (!token || token.trim() === '') {
    throw new Error('Token é obrigatório');
  }

  const response = await fetch(`/api/verificar-email/${token}`, {
    method: 'POST'
  });

  const data = await response.json();

  if (!response.ok) {
    // ✅ Prioridade: message > error
    const errorMsg = data.message || data.error || 'Erro ao verificar email';
    throw new Error(errorMsg);
  }

  return data;
}