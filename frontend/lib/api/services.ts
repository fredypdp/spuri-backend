// src/lib/api/services.ts
// Rotas baseadas em: cmd/server/main.go — revisado em 2026-04

import { api, tokenStorage } from './client';
import type { AsyncBatchResponse } from '@/lib/api/job-service';
import type {
  AuthResponse,
  CriarEscolaRequest,
  CriarUniversidadeRequest,
  LoginRequest,
  CriarEstudanteRequest,
  RegistrarNotasRequest,
  RegistrarFaltasRequest,
  AtualizarFaltaRequest,
  RegistrarAvaliacaoFinalRequest,
  CriarAdminRequest,
  DesativarRequest,
  AtualizarStatusRequest,
  AlterarSenhaRequest,
  SolicitarRecuperacaoRequest,
  MeuPerfilResponse,
  ConsultarEstudanteResponse,
  ConsultarAcademiaResponse,
  ConsultarAcademiasResponse,
  ConsultarEstudantesResponse,
  NotasEstudanteResponse,
  FaltasEstudanteResponse,
  EventosEstudanteResponse,
  VerificarIntegridadeResponse,
  AtualizarStatusResponse,
  CriarCursoRequest,
  AtualizarCursoRequest,
  CriarMateriaRequest,
  AtualizarMateriaRequest,
  ListarCursosResponse,
  ListarMateriasResponse,
  AtualizarDadosPessoaisEstudanteRequest,
  AtualizarDadosAcademiaRequest,
  AtualizarDadosAdminRequest,
  AtualizarRoleAdminRequest,
  RegistroCompleto,
  ListarAdminsResponse,
  DefinirAnoLetivoAcademiaRequest,
  AnoLetivoAcademiaResponse,
  DefinirAnoLetivoResponse,
  AtualizarNotaRequest,
  CriarCategoriaNotaRequest,
  ListarCategoriasNotaResponse,
  ListarTurmasResponse,
  CriarTurmaRequest,
  AtualizarTurmaRequest,
  AdicionarEstudanteTurmaRequest,
  ListarAvaliacoesResponse,
  AvaliacoesEstudanteResponse,
  ListarAprovacoesResponse,
  ListarReprovacoesResponse,
  DefinirPeriodoMateriaRequest,
  DeletarTurmaResponse,
  DeletarCursoResponse,
  TurmasEstudanteResponse,
  Turma,
  AdminDetalhado,
  Curso,
  Materia,
} from '@/types/api';

export interface ErrorResponse {
  error: string;
  message?: string;
  request_id?: string;
}

// =====================
// AUTH (rotas públicas)
// =====================

export const authService = {
  login: (data: LoginRequest) =>
    api.post<AuthResponse>('/login', data),
};

// =====================
// EMAIL
// =====================

export const emailService = {
  verificarEmail: (token: string) =>
    api.post<{ message: string }>(`/email/verificar-email/${token}`, {}),

  solicitarVerificacaoEmail: (data: { email: string }) =>
    api.post<{ message: string }>('/email/verificar-email/solicitar', data),

  solicitarRecuperacaoSenha: (data: SolicitarRecuperacaoRequest) =>
    api.post<{ message: string }>('/email/recuperar-senha/solicitar', data),

  resetarSenha: (token: string, data: { nova_senha: string }) =>
    api.post<{ message: string }>(`/email/recuperar-senha/${token}`, data),

  gerarTokenVerificacao: (data: { email: string }) =>
    api.post<{ message: string }>('/email/gerar-token/verificacao', data),

  gerarTokenRecuperacao: (data: { email: string }) =>
    api.post<{ message: string }>('/email/gerar-token/recuperacao', data),
};

// =====================
// PERFIL
// =====================

export const perfilService = {
  meuPerfil: (token?: string) =>
    api.get<MeuPerfilResponse>('/meu-perfil', {
      token: token || tokenStorage.get() || undefined,
    }),

  alterarSenha: (data: AlterarSenhaRequest, token?: string) =>
    api.put<{ message: string }>(
      '/alterar-senha',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  adicionarTelefoneExtra: (
    data: { numero_telefone: string },
    token?: string
  ) =>
    api.post<{
      message: string;
      id: string;
      numero_telefone: string;
      verificado: boolean;
    }>(
      '/adicionar-telefone-extra',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),
};

// =====================
// CONSULTAS
// =====================

export const consultasService = {
  estudante: (codigoEstudante: string, token?: string) =>
    api.get<ConsultarEstudanteResponse>(
      `/consultar-estudante/${codigoEstudante}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  academia: (codigoAcademia: string, token?: string) =>
    api.get<ConsultarAcademiaResponse>(
      `/consultar-academia/${codigoAcademia}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  listarAcademias: (params?: { limit?: number; offset?: number; status?: 'ativo' | 'inativo'; token?: string }) => {
    const qs = new URLSearchParams();
    if (params?.limit)  qs.append('limit',  params.limit.toString());
    if (params?.offset) qs.append('offset', params.offset.toString());
    if (params?.status) qs.append('status', params.status);
    const query = qs.toString() ? `?${qs.toString()}` : '';
    return api.get<ConsultarAcademiasResponse & { limit: number; offset: number }>(`/academias${query}`, {
      token: params?.token || tokenStorage.get() || undefined,
    });
  },

  listarEstudantes: (token?: string) =>
    api.get<ConsultarEstudantesResponse>('/estudantes', {
      token: token || tokenStorage.get() || undefined,
    }),

  listarAvaliacoes: (params?: { tipo_ensino?: string; token?: string }) => {
    const qs = new URLSearchParams();
    if (params?.tipo_ensino) qs.append('tipo_ensino', params.tipo_ensino);
    const query = qs.toString() ? `?${qs.toString()}` : '';
    return api.get<ListarAvaliacoesResponse>(`/avaliacoes${query}`, {
      token: params?.token || tokenStorage.get() || undefined,
    });
  },

  listarAprovacoes: (token?: string) =>
    api.get<ListarAprovacoesResponse>('/aprovacoes', {
      token: token || tokenStorage.get() || undefined,
    }),

  listarReprovacoes: (token?: string) =>
    api.get<{ reprovacoes: import('@/types/api').AvaliacaoFinal[]; total: number }>('/reprovacoes', {
      token: token || tokenStorage.get() || undefined,
    }),

  notasEstudante: (codigoEstudante: string, token?: string) =>
    api.get<NotasEstudanteResponse>(
      `/notas-estudante/${codigoEstudante}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  faltasEstudante: (codigoEstudante: string, token?: string) =>
    api.get<import('@/types/api').FaltasEstudanteResponse>(
      `/faltas-estudante/${codigoEstudante}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  avaliacoesEstudante: (codigoEstudante: string, token?: string) =>
    api.get<AvaliacoesEstudanteResponse>(
      `/avaliacoes-estudante/${codigoEstudante}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  /**
   * GET /turmas-estudante/:codigo
   * Regras de autorização:
   * - estudante: apenas as próprias turmas
   * - academia: apenas estudantes da própria academia
   * - admin: qualquer estudante
   */
  turmasEstudante: (codigoEstudante: string, token?: string) =>
    api.get<TurmasEstudanteResponse>(
      `/turmas-estudante/${codigoEstudante}`,
      { token: token || tokenStorage.get() || undefined }
    ),
};

// =====================
// EVENT SOURCING
// =====================

export const eventSourcingService = {
  eventosEstudante: (codigoEstudante: string, token?: string) =>
    api.get<EventosEstudanteResponse>(
      `/eventos-estudante/${codigoEstudante}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  verificarIntegridade: (codigoEstudante: string, token?: string) =>
    api.get<VerificarIntegridadeResponse>(
      `/verificar-integridade/${codigoEstudante}`,
      { token: token || tokenStorage.get() || undefined }
    ),
};

// =====================
// ESTUDANTE
// =====================

export const estudanteService = {
  atualizarDadosPessoais: (data: AtualizarDadosPessoaisEstudanteRequest, token?: string) =>
    api.put<{ message: string }>(
      '/estudante/dados-pessoais',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  minhasAvaliacoes: (token?: string) =>
    api.get<ListarAvaliacoesResponse>('/estudante/minhas-avaliacoes', {
      token: token || tokenStorage.get() || undefined,
    }),

  minhasNotas: (token?: string) =>
    api.get<NotasEstudanteResponse>('/estudante/minhas-notas', {
      token: token || tokenStorage.get() || undefined,
    }),

  minhasFaltas: (token?: string) =>
    api.get<import('@/types/api').FaltasEstudanteResponse>('/estudante/minhas-faltas', {
      token: token || tokenStorage.get() || undefined,
    }),
};

// =====================
// ACADEMIA
// =====================

export const academiaService = {
  atualizarDados: (data: AtualizarDadosAcademiaRequest, token?: string) =>
    api.put<{ message: string; aviso?: string; email_verificado?: boolean }>(
      '/academia/dados',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  cadastrarEstudante: (data: CriarEstudanteRequest, token?: string) =>
    api.post<{ message: string; data: { id: string; codigo_estudante: string; codigo_academia: string; status: string } }>(
      '/academia/estudante/register',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Notas ──────────────────────────────────────────────────────────

  registrarNota: (data: RegistrarNotasRequest, token?: string) =>
    api.post<{ message: string; estudante: string; materia: string; nota: number; ano_academico: string; periodo: string; periodos_validos: string[] }>(
      '/academia/notas-aluno',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarNota: (data: AtualizarNotaRequest, token?: string) =>
    api.put<{ message: string; nota_anterior: number; nota_nova: number; observacao: string }>(
      '/academia/atualizar-nota',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarNota: (notaId: string, motivo: string, token?: string) =>
    api.delete<{ message: string }>(
      `/academia/nota/${notaId}`,
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: JSON.stringify({ motivo }),
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  // ── Faltas ──────────────────────────────────────────────────────────

  registrarFaltas: (data: RegistrarFaltasRequest, token?: string) =>
    api.post<{ message: string; estudante: string; materia: string; quantidade: number; ano_academico: string }>(
      '/academia/faltas-aluno',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarFalta: (data: AtualizarFaltaRequest, token?: string) =>
    api.put<{ message: string; id: string; codigo_estudante: string }>(
      '/academia/atualizar-falta',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarFalta: (faltaId: string, motivo: string, token?: string) =>
    api.delete<{ message: string }>(
      `/academia/falta/${faltaId}`,
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: JSON.stringify({ motivo }),
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  // ── Avaliação Final ────────────────────────────────────────────────

  registrarAvaliacaoFinal: (data: RegistrarAvaliacaoFinalRequest, token?: string) =>
    api.post<{
      message: string;
      resultado: string;
      turmas_removidas: string[];
      avisos_turmas?: string[];
    }>(
      '/academia/avaliacao-final',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Ano letivo ────────────────────────────────────────────────────

  definirAnoLetivo: (data: DefinirAnoLetivoAcademiaRequest, token?: string) =>
    api.post<DefinirAnoLetivoResponse>(
      '/academia/ano-letivo',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  getAnoLetivo: (token?: string) =>
    api.get<AnoLetivoAcademiaResponse>(
      '/academia/ano-letivo',
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Categorias de nota ────────────────────────────────────────────

  criarCategoriaNota: (data: CriarCategoriaNotaRequest, token?: string) =>
    api.post<{ message: string; categoria: string }>(
      '/academia/categorias-nota',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  listarCategoriasNota: (token?: string) =>
    api.get<ListarCategoriasNotaResponse>(
      '/academia/categorias-nota',
      { token: token || tokenStorage.get() || undefined }
    ),

  /**
   * DELETE /academia/categorias-nota/:nome
   * Remove (inativa) uma categoria de nota adicional da academia.
   */
  deletarCategoriaNota: (nome: string, token?: string) =>
    api.delete<{ message: string; categoria: string }>(
      `/academia/categorias-nota/${encodeURIComponent(nome)}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Status escolar ────────────────────────────────────────────────

  atualizarStatusEscolarFundamental: (codigoEstudante: string, data: AtualizarStatusRequest, token?: string) =>
    api.put<AtualizarStatusResponse>(
      `/academia/estudante/${codigoEstudante}/status-escolar-fundamental`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarStatusEscolarMedio: (codigoEstudante: string, data: AtualizarStatusRequest, token?: string) =>
    api.put<AtualizarStatusResponse>(
      `/academia/estudante/${codigoEstudante}/status-escolar-medio`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarStatusSuperior: (codigoEstudante: string, data: AtualizarStatusRequest, token?: string) =>
    api.put<AtualizarStatusResponse>(
      `/academia/estudante/${codigoEstudante}/status-superior`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Cursos ────────────────────────────────────────────────────────

  criarCurso: (data: CriarCursoRequest, token?: string) =>
    api.post<{ message: string; data: { id: string; nome: string; type: string; periodos?: string[] } }>(
      '/academia/curso',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  listarCursos: (token?: string) =>
    api.get<ListarCursosResponse>('/academia/cursos', {
      token: token || tokenStorage.get() || undefined,
    }),

  getCurso: (cursoId: string, token?: string) =>
    api.get<Curso>(
      `/academia/curso/${cursoId}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  ativarCurso: (cursoId: string, token?: string) =>
    api.put<{ message: string; nome: string }>(
      `/academia/curso/${cursoId}/ativar`,
      undefined,
      { token: token || tokenStorage.get() || undefined }
    ),

  desativarCurso: (cursoId: string, token?: string) =>
    api.put<{ message: string; nome: string }>(
      `/academia/curso/${cursoId}/desativar`,
      undefined,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarCurso: (cursoId: string, data: AtualizarCursoRequest, token?: string) =>
    api.put<{ message: string; nome: string; type: string; anos_academicos: string[]; periodos?: string[] }>(
      `/academia/curso/${cursoId}/dados`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarCurso: (cursoId: string, motivo?: string, token?: string) =>
    api.delete<DeletarCursoResponse>(
      `/academia/curso/${cursoId}`,
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: motivo ? JSON.stringify({ motivo }) : undefined,
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  // ── Matérias ────────────────────────────────────────────────────────

  criarMateria: (data: CriarMateriaRequest, token?: string) =>
    api.post<{ message: string; data: { id: string; nome: string; type: string; status: string; proximo_passo?: string } }>(
      '/academia/materia',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  listarMaterias: (token?: string) =>
    api.get<ListarMateriasResponse>('/academia/materias', {
      token: token || tokenStorage.get() || undefined,
    }),

  getMateria: (materiaId: string, token?: string) =>
    api.get<Materia>(
      `/academia/materia/${materiaId}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  ativarMateria: (materiaId: string, token?: string) =>
    api.put<{ message: string; nome: string }>(
      `/academia/materia/${materiaId}/ativar`,
      undefined,
      { token: token || tokenStorage.get() || undefined }
    ),

  desativarMateria: (materiaId: string, token?: string) =>
    api.put<{ message: string; nome: string }>(
      `/academia/materia/${materiaId}/desativar`,
      undefined,
      { token: token || tokenStorage.get() || undefined }
    ),

  definirPeriodoMateria: (materiaId: string, data: DefinirPeriodoMateriaRequest, token?: string) =>
    api.put<{ message: string; nome: string; periodo: string }>(
      `/academia/materia/${materiaId}/periodo`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarMateria: (materiaId: string, data: AtualizarMateriaRequest, token?: string) =>
    api.put<{ message: string; nome: string }>(
      `/academia/materia/${materiaId}/dados`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarMateria: (materiaId: string, token?: string) =>
    api.delete<{ message: string; nome: string }>(
      `/academia/materia/${materiaId}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Turmas ────────────────────────────────────────────────────────

  criarTurma: (data: CriarTurmaRequest, token?: string) =>
    api.post<{ message: string; id: string; codigo_turma: string }>(
      '/academia/turma',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  listarTurmas: (token?: string) =>
    api.get<ListarTurmasResponse>('/academia/turmas', {
      token: token || tokenStorage.get() || undefined,
    }),

  getTurma: (codigoTurma: string, token?: string) =>
    api.get<Turma>(
      `/academia/turma/${codigoTurma}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  ativarTurma: (codigoTurma: string, token?: string) =>
    api.put<{ message: string; codigo_turma: string }>(
      `/academia/turma/${codigoTurma}/ativar`,
      undefined,
      { token: token || tokenStorage.get() || undefined }
    ),

  desativarTurma: (codigoTurma: string, token?: string) =>
    api.put<{ message: string; codigo_turma: string }>(
      `/academia/turma/${codigoTurma}/desativar`,
      undefined,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarTurma: (codigoTurma: string, data: AtualizarTurmaRequest, token?: string) =>
    api.put<{ message: string }>(
      `/academia/turma/${codigoTurma}/dados`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarTurma: (codigoTurma: string, motivo?: string, token?: string) =>
    api.delete<DeletarTurmaResponse>(
      `/academia/turma/${codigoTurma}`,
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: motivo ? JSON.stringify({ motivo }) : undefined,
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  adicionarEstudanteATurma: (codigoTurma: string, data: AdicionarEstudanteTurmaRequest, token?: string) =>
    api.post<{ message: string; codigo_turma: string; codigo_estudante: string }>(
      `/academia/turma/${codigoTurma}/estudante`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  removerEstudanteDaTurma: (codigoTurma: string, codigoEstudante: string, token?: string) =>
    api.delete<{ message: string; codigo_turma: string; codigo_estudante: string }>(
      `/academia/turma/${codigoTurma}/estudantes/${codigoEstudante}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Async — estudantes ──────────────────────────────────────────────

  cadastrarEstudanteBatchAsync: (data: CriarEstudanteRequest[], token?: string) =>
    api.post<AsyncBatchResponse>(
      '/academia/estudante/register/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Async — notas ──────────────────────────────────────────────────

  registrarNotaBatchAsync: (data: RegistrarNotasRequest[], token?: string) =>
    api.post<AsyncBatchResponse>(
      '/academia/notas-aluno/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarNotaBatchAsync: (data: AtualizarNotaRequest[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/atualizar-nota/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarNotaBatchAsync: (data: { id: string; motivo: string }[], token?: string) =>
    api.delete<AsyncBatchResponse>(
      '/academia/nota/async',
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: JSON.stringify(data),
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  // ── Async — faltas ────────────────────────────────────────────────

  registrarFaltasBatchAsync: (data: RegistrarFaltasRequest[], token?: string) =>
    api.post<AsyncBatchResponse>(
      '/academia/faltas-aluno/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarFaltaBatchAsync: (data: AtualizarFaltaRequest[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/atualizar-falta/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarFaltaBatchAsync: (data: { id: string; motivo: string }[], token?: string) =>
    api.delete<AsyncBatchResponse>(
      '/academia/falta/async',
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: JSON.stringify(data),
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  // ── Async — avaliações ────────────────────────────────────────────

  registrarAvaliacaoFinalBatchAsync: (data: RegistrarAvaliacaoFinalRequest[], token?: string) =>
    api.post<AsyncBatchResponse>(
      '/academia/avaliacao-final/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Async — status escolar ────────────────────────────────────────

  atualizarStatusEscolarBatchAsync: (
    data: { codigo_estudante: string; tipo: 'fundamental' | 'medio' | 'superior'; novo_status: string }[],
    token?: string
  ) =>
    api.put<AsyncBatchResponse>(
      '/academia/estudante/status-escolar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Async — cursos ────────────────────────────────────────────────

  criarCursoBatchAsync: (data: CriarCursoRequest[], token?: string) =>
    api.post<AsyncBatchResponse>(
      '/academia/curso/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  ativarCursoBatchAsync: (data: { id: string }[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/curso/ativar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  desativarCursoBatchAsync: (data: { id: string }[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/curso/desativar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarCursoBatchAsync: (data: (AtualizarCursoRequest & { id: string })[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/curso/dados/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarCursoBatchAsync: (data: { id: string; motivo?: string }[], token?: string) =>
    api.delete<AsyncBatchResponse>(
      '/academia/curso/async',
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: JSON.stringify(data),
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  // ── Async — matérias ──────────────────────────────────────────────

  criarMateriaBatchAsync: (data: CriarMateriaRequest[], token?: string) =>
    api.post<AsyncBatchResponse>(
      '/academia/materia/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  ativarMateriaBatchAsync: (data: { id: string }[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/materia/ativar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  desativarMateriaBatchAsync: (data: { id: string }[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/materia/desativar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  definirPeriodoMateriaBatchAsync: (data: (DefinirPeriodoMateriaRequest & { id: string })[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/materia/periodo/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarMateriaBatchAsync: (data: ({ id: string } & AtualizarMateriaRequest)[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/materia/dados/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarMateriaBatchAsync: (data: { id: string }[], token?: string) =>
    api.delete<AsyncBatchResponse>(
      '/academia/materia/async',
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: JSON.stringify(data),
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  // ── Async — turmas ────────────────────────────────────────────────

  criarTurmaBatchAsync: (data: CriarTurmaRequest[], token?: string) =>
    api.post<AsyncBatchResponse>(
      '/academia/turma/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  ativarTurmaBatchAsync: (data: { codigo_turma: string }[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/turma/ativar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  desativarTurmaBatchAsync: (data: { codigo_turma: string }[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/turma/desativar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarTurmaBatchAsync: (data: ({ codigo_turma: string } & AtualizarTurmaRequest)[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/turma/dados/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarTurmaBatchAsync: (data: { codigo_turma: string; motivo?: string }[], token?: string) =>
    api.delete<AsyncBatchResponse>(
      '/academia/turma/async',
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: JSON.stringify(data),
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  adicionarEstudanteBatchAsync: (
    data: { codigo_turma: string; codigo_estudante: string }[],
    token?: string
  ) =>
    api.post<AsyncBatchResponse>(
      '/academia/turma/estudante/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  removerEstudanteTurmaBatchAsync: (
    data: { codigo_turma: string; codigo_estudante: string }[],
    token?: string
  ) =>
    api.delete<AsyncBatchResponse>(
      '/academia/turma/estudante/async',
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: JSON.stringify(data),
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),

  // ── Async — dados da academia ─────────────────────────────────────

  atualizarAcademiaBatchAsync: (data: AtualizarDadosAcademiaRequest[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/academia/dados/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Async — categorias de nota ────────────────────────────────────

  criarCategoriasNotaBatchAsync: (data: CriarCategoriaNotaRequest[], token?: string) =>
    api.post<AsyncBatchResponse>(
      '/academia/categorias-nota/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  deletarCategoriasNotaBatchAsync: (data: { nome: string }[], token?: string) =>
    api.delete<AsyncBatchResponse>(
      '/academia/categorias-nota/async',
      {
        token: token || tokenStorage.get() || undefined,
        method: 'DELETE',
        body: JSON.stringify(data),
        headers: { 'Content-Type': 'application/json' },
      } as any
    ),
};

// =====================
// ADMIN
// =====================

export const adminService = {
  criarAdmin: (data: CriarAdminRequest, token?: string) =>
    api.post<{ message: string; data: AdminDetalhado; aviso?: string }>(
      '/dominis/register',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  registrarAcademia: (data: CriarEscolaRequest | CriarUniversidadeRequest, token?: string) =>
    api.post<{ message: string; codigo_academia: string; data: { codigo_academia: string; id: string; nome: string; provincia: string } }>(
      '/dominis/academia/register',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  ativarAcademia: (codigoAcademia: string, token?: string) =>
    api.put<{ message: string }>(
      `/dominis/academia/${codigoAcademia}/ativar`,
      undefined,
      { token: token || tokenStorage.get() || undefined }
    ),

  desativarAcademia: (codigoAcademia: string, data: DesativarRequest, token?: string) =>
    api.put<{ message: string }>(
      `/dominis/academia/${codigoAcademia}/desativar`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  ativarAdmin: (adminId: string, token?: string) =>
    api.put<{ message: string; email: string }>(
      `/dominis/admin/${adminId}/ativar`,
      undefined,
      { token: token || tokenStorage.get() || undefined }
    ),

  desativarAdmin: (adminId: string, data: DesativarRequest, token?: string) =>
    api.put<{ message: string; email: string }>(
      `/dominis/admin/${adminId}/desativar`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  listarAdmins: (token?: string) =>
    api.get<ListarAdminsResponse>('/dominis/admin-lista', {
      token: token || tokenStorage.get() || undefined,
    }),

  getMetrics: (token?: string) =>
    api.get<{ metrics: Record<string, unknown> }>('/dominis/metrics', {
      token: token || tokenStorage.get() || undefined,
    }),

  /**
   * Rebuild síncrono — mantido para compatibilidade.
   * Para projeções com alto volume, prefira rebuildProjectionAsync.
   */
  rebuildProjection: (name: string, token?: string) =>
    api.post<{ message: string; projection: string }>(
      `/dominis/projections/rebuild/${name}`,
      {},
      { token: token || tokenStorage.get() || undefined }
    ),

  /**
   * Rebuild assíncrono — retorna 202 com job_id, poll_url e sse_url.
   * Acompanhe via GET /jobs/:id (polling) ou GET /jobs/stream (SSE).
   *
   * Desde a versão 1.0.9 do backend, a resposta inclui `sse_url`
   * padronizado junto com `poll_url`.
   */
  rebuildProjectionAsync: (name: string, token?: string) =>
    api.post<AsyncBatchResponse>(
      `/dominis/projections/rebuild/${name}/async`,
      {},
      { token: token || tokenStorage.get() || undefined }
    ),

  consultarAdminPorEmail: (email: string, token?: string) =>
    api.get<{ admin: AdminDetalhado }>(
      `/dominis/consultar-admin/${encodeURIComponent(email)}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  listarTodosRegistros: (params?: { limit?: number; offset?: number; tipo?: string; token?: string }) => {
    const qs = new URLSearchParams();
    if (params?.limit)  qs.append('limit',  params.limit.toString());
    if (params?.offset) qs.append('offset', params.offset.toString());
    if (params?.tipo)   qs.append('tipo',   params.tipo);
    const query = qs.toString() ? `?${qs.toString()}` : '';
    return api.get<RegistroCompleto>(`/dominis/registros${query}`, {
      token: params?.token || tokenStorage.get() || undefined,
    });
  },

  registrosPorEstudante: (codigoEstudante: string, params?: { limit?: number; offset?: number; token?: string }) => {
    const qs = new URLSearchParams();
    if (params?.limit)  qs.append('limit',  params.limit.toString());
    if (params?.offset) qs.append('offset', params.offset.toString());
    const query = qs.toString() ? `?${qs.toString()}` : '';
    return api.get<RegistroCompleto>(`/dominis/registros/${codigoEstudante}${query}`, {
      token: params?.token || tokenStorage.get() || undefined,
    });
  },

  atualizarRoleAdmin: (adminId: string, data: AtualizarRoleAdminRequest, token?: string) =>
    api.put<{ message: string; role_anterior: string; novo_role: string }>(
      `/dominis/admin/${adminId}/role`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  atualizarDadosAdmin: (adminId: string, data: AtualizarDadosAdminRequest, token?: string) =>
    api.put<{ message: string }>(
      `/dominis/admin/${adminId}/dados`,
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Async — academias ─────────────────────────────────────────────

  registrarAcademiaBatchAsync: (
    data: (CriarEscolaRequest | CriarUniversidadeRequest)[],
    token?: string
  ) =>
    api.post<AsyncBatchResponse>(
      '/dominis/academia/register/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  ativarAcademiaBatchAsync: (codigos: string[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/dominis/academia/ativar/async',
      codigos.map(codigo => ({ codigo })),
      { token: token || tokenStorage.get() || undefined }
    ),

  desativarAcademiaBatchAsync: (
    data: { codigo: string; motivo: string }[],
    token?: string
  ) =>
    api.put<AsyncBatchResponse>(
      '/dominis/academia/desativar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Async — admins ────────────────────────────────────────────────

  /**
   * PUT /dominis/admin/ativar/async
   * Ativa múltiplos admins em lote.
   */
  ativarAdminBatchAsync: (data: { id: string }[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/dominis/admin/ativar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  /**
   * PUT /dominis/admin/desativar/async
   * Desativa múltiplos admins em lote.
   */
  desativarAdminBatchAsync: (data: { id: string; motivo: string }[], token?: string) =>
    api.put<AsyncBatchResponse>(
      '/dominis/admin/desativar/async',
      data,
      { token: token || tokenStorage.get() || undefined }
    ),

  // ── Jobs ─────────────────────────────────────────────────────────

  listarJobs: (token?: string) =>
    api.get<{ jobs: import('@/lib/api/job-service').JobSummary[]; total: number }>(
      '/jobs',
      { token: token || tokenStorage.get() || undefined }
    ),

  consultarJob: (jobId: string, token?: string) =>
    api.get<import('@/lib/api/job-service').JobSummary>(
      `/jobs/${jobId}`,
      { token: token || tokenStorage.get() || undefined }
    ),

  consultarJobComResultados: (jobId: string, token?: string) =>
    api.get<{ job: import('@/lib/api/job-service').JobSummary; results: import('@/lib/api/job-service').JobItemResult[] }>(
      `/jobs/${jobId}?results=true`,
      { token: token || tokenStorage.get() || undefined }
    ),
};