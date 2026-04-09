// src/lib/api/services/email.service.ts

import { api } from '../client';
import type {
  GerarTokenVerificacaoRequest,
  GerarTokenRecuperacaoRequest,
  TokenResponse,
  VerificarEmailResponse,
  ResetarSenhaResponse,
} from '@/types/email-auth';

class EmailAuthService {

  // Requer JWT — usuário logado solicitando verificação do próprio email
  async gerarTokenVerificacao(
    data: GerarTokenVerificacaoRequest,
    authHeader?: string  // ← "Bearer <token>" vindo do route.ts
  ): Promise<TokenResponse> {
    // Extrai o token puro do header "Bearer <token>"
    const token = authHeader?.startsWith('Bearer ')
      ? authHeader.slice(7)
      : authHeader;

    return api.post<TokenResponse>(
      '/email/gerar-token/verificacao',
      {}, // body vazio — o backend usa o JWT para identificar o usuário
      { token } // ← passa via FetchOptions para o Authorization header
    );
  }

  // Pública — usuário esqueceu a senha, passa identificador + tipo no body
  async gerarTokenRecuperacao(data: GerarTokenRecuperacaoRequest): Promise<TokenResponse> {
    return api.post<TokenResponse>('/email/gerar-token/recuperacao', {
      identificador: data.identificador,
      tipo: data.tipo,
    });
  }

  async verificarEmail(token: string): Promise<VerificarEmailResponse> {
    return api.post<VerificarEmailResponse>(`/email/verificar-email/${token}`, {});
  }

  async resetarSenha(token: string): Promise<ResetarSenhaResponse> {
    return api.post<ResetarSenhaResponse>(`/email/recuperar-senha/${token}`, {});
  }
}

export const emailAuthService = new EmailAuthService();