// src/types/email-auth.ts
import { UserType } from "./api";

/**
 * Request para gerar token de verificação (frontend envia email)
 */
export interface GerarTokenVerificacaoRequest {
  identificador: string; // codigo_estudante, codigo_academia ou email (admin)
  tipo: UserType; // 'estudante' | 'academia' | 'admin'
}

/**
 * Request para gerar token de recuperação (frontend envia email)
 */
export interface GerarTokenRecuperacaoRequest {
  identificador: string; // codigo_estudante, codigo_academia ou email (admin)
  tipo: UserType; // 'estudante' | 'academia' | 'admin'
}

/**
 * Response do backend quando gera token (frontend deve enviar email)
 */
export interface TokenResponse {
  success: boolean;
  token: string;
  email: string;
  nome: string;
  tipo: string;
  expira_em: string;
}

/**
 * Response quando email é verificado com sucesso
 */
export interface VerificarEmailResponse {
  message: string;
  email: string;
}

/**
 * Response quando senha é resetada
 * Inclui a senha padrão gerada pelo backend
 */
export interface ResetarSenhaResponse {
  message: string;
  senha_padrao: string;  // Senha gerada pelo backend
  email: string;
  proximos_passos: string;
}

/**
 * ✅ Response de erro quando email não está verificado
 */
export interface EmailNaoVerificadoError {
  error: "email não verificado";
  message: string;
  email_verificado: false;
}

/**
 * Request para alterar senha (usuário logado)
 */
export interface AlterarSenhaRequest {
  senha_atual: string;
  nova_senha: string;
}

/**
 * Response após alterar senha
 */
export interface AlterarSenhaResponse {
  message: string;
}

/**
 * Senhas padrão por tipo de usuário
 * Baseado em: internal/services/email_service.go -> GetDefaultPassword()
 */
export const SenhasPadrao = {
  estudante: (codigo: string) => codigo,           // codigo_estudante
  academia: (codigo: string) => codigo,            // codigo_academia
  admin: "spuriadm",
  gerente: "spurigerente",
  fpp: "spurifpp",
  default: "spuri123"
} as const;

/**
 * Tipo para as senhas padrão
 */
export type SenhaPadrao = 
  | string                    // Para estudante/academia (dinâmico)
  | "spuriadm"               // Admin
  | "spurigerente"           // Gerente
  | "spurifpp"               // FPP
  | "spuri123";              // Default

/**
 * Helper para obter senha padrão
 */
export function getSenhaPadrao(userType: UserType, codigo?: string, role?: string): string {
  switch (userType) {
    case "estudante":
      return codigo || SenhasPadrao.default;
    case "academia":
      return codigo || SenhasPadrao.default;
    case "admin":
      if (role === "fpp") return SenhasPadrao.fpp;
      if (role === "gerente") return SenhasPadrao.gerente;
      return SenhasPadrao.admin; // "adm" e default
    default:
      return SenhasPadrao.default;
  }
}

/**
 * Response do endpoint /api/verificar-email (Next.js API route)
 */
export interface FrontendEmailVerificacaoResponse {
  success: boolean;
  message: string;
  messageId?: string;
  error?: string;
}

/**
 * Response do endpoint /api/recuperar-senha (Next.js API route)
 */
export interface FrontendEmailRecuperacaoResponse {
  success: boolean;
  message: string;
  messageId?: string;
  error?: string;
}

/**
 * ✅ Type guard para verificar se erro é EmailNaoVerificadoError
 */
export function isEmailNaoVerificadoError(error: any): error is EmailNaoVerificadoError {
  return (
    error &&
    error.error === "email não verificado" &&
    error.email_verificado === false
  );
}