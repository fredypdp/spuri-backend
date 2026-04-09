// src/types/api.ts

// =====================
// BASE TYPES
// =====================

export type UserType = 'academia' | 'estudante' | 'admin';
export type AdminType = 'gerente' | 'adm' | 'fpp';

export type AcademiaType = 'escola' | 'superior';
export type NivelEscolar = 'fundamental' | 'medio' | 'misto';

export type AnoAcademico = AnoFundamental | AnoMedio | AnoSuperior;

/**
 * Anos do ensino fundamental. Formato: [1-9]_ano_fundamental
 */
export type AnoFundamental =
  | '1_ano_fundamental' | '2_ano_fundamental' | '3_ano_fundamental'
  | '4_ano_fundamental' | '5_ano_fundamental' | '6_ano_fundamental'
  | '7_ano_fundamental' | '8_ano_fundamental' | '9_ano_fundamental';

/**
 * Anos do ensino médio. Formato: [n]_ano_medio
 */
export type AnoMedio =
  | '1_ano_medio' | '2_ano_medio' | '3_ano_medio' | '4_ano_medio';

/**
 * Anos do ensino superior. Formato: [n]_ano_superior
 */
export type AnoSuperior =
  | '1_ano_superior' | '2_ano_superior' | '3_ano_superior'
  | '4_ano_superior' | '5_ano_superior' | '6_ano_superior';

/** @deprecated Use AnoFundamental | AnoMedio | AnoSuperior separadamente. */
export type AnoEscolar = AnoFundamental | AnoMedio;

export type StatusEscolar            = 'inativo' | 'em_andamento' | 'finalizado';
export type StatusEscolarFundamental = StatusEscolar;
export type StatusEscolarMedio       = StatusEscolar;
export type StatusSuperior           = 'inativo' | 'em_andamento' | 'finalizado';
export type StatusGeral              = 'inativo' | 'ativo' | 'finalizado';

/**
 * Períodos letivos.
 * - Escolar (fixo): 1_trimestre, 2_trimestre, 3_trimestre
 * - Superior (dinâmico): [n]_semestre
 */
export type Periodo =
  | '1_trimestre' | '2_trimestre' | '3_trimestre'
  | `${number}_semestre`;

export type CursoType   = 'medio' | 'superior';
export type MateriaType = 'fundamental' | 'medio' | 'superior';
export type Genero      = 'masculino' | 'feminino';

export type TipoEnsino = 'fundamental' | 'medio' | 'superior';

// =====================
// REQUEST TYPES
// =====================

export interface CriarEscolaRequest {
  type: 'escola';
  nome: string;
  provincia: string;
  endereco: string;
  numero_telefone?: string;
  email?: string;
  website?: string;
  nivel_escolar: NivelEscolar;
  cursos?: string[];
  anos_academicos?: string[];
}

export interface CriarUniversidadeRequest {
  type: 'superior';
  nome: string;
  provincia: string;
  endereco: string;
  numero_telefone?: string;
  email?: string;
  website?: string;
  cursos?: string[];
}

export interface LoginRequest {
  usuario: string;
  senha: string;
}

export interface CriarEstudanteRequest {
  nome: string;
  /** Obrigatório. "masculino" | "feminino" */
  genero: Genero;
  /** Obrigatório. Formato ISO: "YYYY-MM-DD". */
  data_nascimento: string;
  email?: string;
  telefone?: string;
  bilhete_identidade?: string;
  bilhete_identidade_responsavel?: string;
  ano_escolar?: string;
  status_escolar_fundamental?: StatusEscolarFundamental;
  ano_escolar_medio?: string;
  status_escolar_medio?: StatusEscolarMedio;
  curso_medio_id?: string;
  ano_superior?: string;
  status_superior?: StatusSuperior;
  curso_superior_id?: string;
}

export interface RegistrarFaltasRequest {
  codigo_estudante: string;
  ano_academico?: string;
  data: string;
  materia_disciplinar_id: string;
  quantidade: number;
  observacao?: string;
}

export interface CriarAdminRequest {
  nome: string;
  email: string;
  role: AdminType;
}

export interface DesativarRequest {
  motivo: string;
}

export interface AtualizarStatusRequest {
  novo_status: StatusEscolar | StatusSuperior;
}

export interface CriarCursoRequest {
  nome: string;
  type: CursoType;
  anos_academicos: string[];
  /** Obrigatório para superior */
  periodos?: string[];
}

export interface AtualizarCursoRequest {
  nome?: string;
  anos_academicos?: string[];
  periodos?: string[];
}

export interface CriarMateriaRequest {
  nome: string;
  type: MateriaType;
  anos_academicos?: string[];
  curso_id?: string;
}

export interface AtualizarMateriaRequest {
  nome?: string;
}

/** PUT /academia/materias/:id/periodo */
export interface DefinirPeriodoMateriaRequest {
  periodo: string;
}

export interface AtualizarDadosPessoaisEstudanteRequest {
  nome?: string;
  email?: string;
  telefone?: string;
  bilhete_identidade?: string;
  bilhete_identidade_responsavel?: string;
  data_nascimento?: string;
}

export interface AtualizarDadosAcademicosEstudanteRequest {
  ano_escolar?: string;
  ano_escolar_medio?: string;
  ano_superior?: string;
  curso_medio_id?: string;
  curso_superior_id?: string;
}

export interface AtualizarDadosAcademiaRequest {
  nome?: string;
  provincia?: string;
  endereco?: string;
  numero_telefone?: string;
  email?: string;
  website?: string;
  nivel_escolar?: string;
  cursos?: string[];
}

export interface AtualizarDadosAdminRequest {
  nome?: string;
  email?: string;
}

export interface AtualizarRoleAdminRequest {
  novo_role: AdminType;
}

export interface AlterarSenhaRequest {
  senha_atual: string;
  nova_senha: string;
}

export interface SolicitarRecuperacaoRequest {
  identificador: string;
  tipo: UserType;
}

export interface SolicitarVerificacaoRequest {
  identificador: string;
  tipo: UserType;
}

export interface AlterarCursoRequest {
  curso_id: string;
  tipo_ensino: 'medio' | 'superior';
}

/**
 * POST /academia/avaliacao-final
 * Campos alinhados com a API: nivel_ano_academico_atual, proximo_ano_academico
 */
export interface RegistrarAvaliacaoFinalRequest {
  codigo_estudante: string;
  tipo_ensino: TipoEnsino;
  /** Nível académico atual (ex: '3_ano_fundamental', '2_ano_medio') */
  nivel_ano_academico_atual: string;
  /** Próximo nível (obrigatório se aprovado e não for o último ano) */
  proximo_ano_academico?: string;
  aprovado: boolean;
  observacao?: string;
}

/**
 * @deprecated Use RegistrarAvaliacaoFinalRequest.
 */
export interface RegistrarAprovacaoAnoRequest {
  codigo_estudante: string;
  tipo_ensino: TipoEnsino;
  nivel_atual: string;
  proximo_nivel?: string;
  aprovado: boolean;
  observacao?: string;
}

// ── Notas ────────────────────────────────────────────────────────────────────

export type TipoNota = 'escolar' | 'superior';

export type CategoriaNotaEscolar =
  | 'nota_escola'
  | 'nota_professor';

export type CategoriaNotaSuperiorFixa =
  | 'nota_pp1'
  | 'nota_pp2'
  | 'nota_exame';

export type CategoriaNota =
  | CategoriaNotaEscolar
  | CategoriaNotaSuperiorFixa
  | string;

export interface RegistrarNotasRequest {
  codigo_estudante: string;
  periodo: Periodo;
  materia_disciplinar_id: string;
  tipo: TipoNota;
  categoria: CategoriaNota;
  nota: number;
  observacao?: string;
}

export interface AtualizarNotaRequest {
  id: string;
  nota_nova: number;
  observacao: string;
}

export interface AtualizarFaltaRequest {
  id: string;
  data?: string;
  materia_disciplinar_id?: string;
  quantidade?: number;
  observacao?: string;
}

export interface CriarCategoriaNotaRequest {
  nome: string;
  descricao?: string;
}

// ── Turmas ───────────────────────────────────────────────────────────────────

export interface CriarTurmaRequest {
  codigo_turma: string;
  nivel: string;
  turno: 'manha' | 'tarde' | 'noite';
  curso_id?: string;
}

export interface AtualizarTurmaRequest {
  nivel?: string;
  turno?: string;
  curso_id?: string;
}

export interface AdicionarEstudanteTurmaRequest {
  codigo_estudante: string;
}

// ── Ano Letivo ────────────────────────────────────────────────────────────────

/**
 * POST /academia/ano-letivo
 * tipo: 'escola' | 'superior'
 */
export interface DefinirAnoLetivoAcademiaRequest {
  ano_letivo: string; // formato: YYYY_YYYY  ex: "2025_2026"
  tipo: 'escola' | 'superior';
}

/**
 * @deprecated Substituído por DefinirAnoLetivoAcademiaRequest.
 */
export interface DefinirAnoLetivoRequest {
  ano_letivo: string;
}

// =====================
// RESPONSE TYPES
// =====================

export interface AuthResponse {
  token: string;
  nome: string;
  type: UserType;
  codigo?: string;
  email?: string;
  role?: AdminType;
}

export interface ApiResponse<T> {
  data: T;
  message?: string;
}

// ── Modelos de domínio ────────────────────────────────────────────────────────

export interface Nota {
  id: string;
  codigo_estudante: string;
  codigo_academia: string;
  ano_lectivo: string;
  ano_academico?: string;
  periodo: Periodo;
  materia_disciplinar_id: string;
  materia_nome?: string;
  tipo: TipoNota;
  categoria: CategoriaNota;
  nota: number;
  observacao?: string;
  registered_at: string;
  event_id: string;
  version: number;
}

export interface Falta {
  id: string;
  codigo_estudante: string;
  codigo_academia: string;
  ano_lectivo: string;
  ano_academico: string;
  data: string;
  materia_disciplinar_id: string;
  materia_nome?: string;
  quantidade: number;
  observacao?: string;
  registered_at: string;
  event_id: string;
  version: number;
}

export interface AprovacaoAno {
  id: string;
  codigo_estudante: string;
  codigo_academia: string;
  ano_lectivo: string;
  tipo_ensino: TipoEnsino;
  nivel_atual: string;
  proximo_nivel?: string;
  aprovado: boolean;
  observacao?: string;
  registered_at: string;
  event_id: string;
  version: number;
}

/**
 * Avaliação final — projecção projection_avaliacao_final.
 * Campos alinhados com AvaliacaoFinalDTO do backend.
 */
export interface AvaliacaoFinal {
  id: string;
  event_id: string;
  codigo_estudante: string;
  codigo_academia: string;
  ano_lectivo: string;
  tipo_ensino: TipoEnsino;
  /** Campo real da projecção: ano_academico_atual */
  ano_academico_atual: string;
  proximo_ano_academico?: string;
  aprovado: boolean;
  observacao?: string;
  registered_at: string;
  version: number;
}

export interface Curso {
  id: string;
  nome: string;
  type: CursoType;
  anos_academicos: string[];
  periodos?: string[];
  codigo_academia: string;
  status: 'ativo' | 'inativo' | 'deletado';
  deleted_at?: string;
  created_at: string;
  updated_at: string;
  version: number;
}

export interface Materia {
  id: string;
  nome: string;
  type: MateriaType;
  anos_academicos?: string[];
  codigo_academia: string;
  curso_id?: string;
  /** Apenas para tipo 'superior'. */
  periodo?: string;
  status: 'ativo' | 'inativo' | 'deletado';
  deleted_at?: string;
  created_at: string;
  updated_at: string;
  version: number;
}

export interface Turma {
  id: string;
  codigo_turma: string;
  codigo_academia: string;
  nivel: string;
  curso_id?: string;
  turno: 'manha' | 'tarde' | 'noite';
  estudantes: string[];
  /**
   * Histórico de estudantes por ano letivo.
   * Chave: ano_letivo (ex: "2025_2026") → valor: lista de codigo_estudante
   * que já fizeram parte desta turma nesse ano letivo.
   * Populado ao adicionar estudante e na remoção automática via avaliação final.
   */
  historico_estudantes_ano_letivo?: Record<string, string[]>;
  status: 'ativo' | 'inativo' | 'deletado';
  status_alterado_por?: string;
  status_alterado_em?: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
  version: number;
}

export interface CategoriaNotaItem {
  id: string;
  codigo_academia: string;
  nome: string;
  descricao?: string;
  adicionado_por?: string;
  status: 'ativo' | 'inativo';
  created_at: string;
  version: number;
}

export interface Evento {
  id: number;
  event_id: string;
  aggregate_id: string;
  aggregate_type: string;
  event_type: string;
  event_version: number;
  payload: any;
  metadata: any;
  occurred_at: string;
  recorded_at: string;
  ledger_hash: string;
  previous_hash?: string;
}

// ── Perfis ────────────────────────────────────────────────────────────────────

export interface EstudanteDetalhado {
  id: string;
  nome: string;
  codigo_estudante: string;
  email?: string;
  telefone?: string;
  email_verificado: boolean;
  bilhete_identidade?: string;
  bilhete_identidade_responsavel?: string;
  genero: Genero;
  data_nascimento: string;
  codigo_academia?: string;
  status: StatusGeral;
  status_escolar_fundamental: StatusEscolar;
  status_escolar_medio: StatusEscolar;
  status_superior: StatusSuperior;
  ano_escolar?: string;
  ano_escolar_medio?: string;
  ano_superior?: string;
  curso_medio_id?: string;
  curso_superior_id?: string;
  total_notas?: number;
  total_faltas?: number;
  created_at: string;
  updated_at: string;
  version: number;
}

export interface AcademiaDetalhada {
  id: string;
  type: AcademiaType;
  nome: string;
  codigo_academia: string;
  provincia: string;
  endereco: string;
  numero_telefone?: string;
  email?: string;
  email_verificado: boolean;
  website?: string;
  nivel_escolar?: NivelEscolar;
  anos_academicos?: string[];
  status: string;
  cursos: string[];
  created_at: string;
  updated_at?: string;
  total_estudantes: number;
  version: number;
  ano_letivo?: string;
  tipo_ano_letivo?: 'escola' | 'superior';
  ano_letivo_ativado_em?: string;
}

export interface AdminDetalhado {
  id: string;
  nome: string;
  email: string;
  email_verificado: boolean;
  role: AdminType;
  status: string;
  created_at: string;
}

// ── Respostas de listagem ─────────────────────────────────────────────────────

export interface ListarCursosResponse {
  cursos: Curso[];
  total: number;
}

export interface ListarMateriasResponse {
  materias: Materia[];
  total: number;
}

export interface ListarTurmasResponse {
  turmas: Turma[];
}

export interface ListarCategoriasNotaResponse {
  categorias: CategoriaNotaItem[];
  total: number;
}

export interface NotasEstudanteResponse {
  codigo_estudante: string;
  nome: string;
  notas: Nota[];
  total: number;
}

export interface FaltasEstudanteResponse {
  codigo_estudante: string;
  nome: string;
  faltas: Falta[];
  total: number;
}

export interface AprovacoesEstudanteResponse {
  codigo_estudante: string;
  nome: string;
  aprovacoes: AprovacaoAno[];
  total: number;
}

/** GET /avaliacoes e GET /estudante/minhas-avaliacoes */
export interface ListarAvaliacoesResponse {
  avaliacoes: AvaliacaoFinal[];
  total: number;
}

/** GET /avaliacoes-estudante/:codigo */
export interface AvaliacoesEstudanteResponse {
  codigo_estudante: string;
  nome: string;
  avaliacoes: AvaliacaoFinal[];
  total: number;
}

/** GET /aprovacoes */
export interface ListarAprovacoesResponse {
  aprovacoes: AvaliacaoFinal[];
  total: number;
}

/** GET /reprovacoes */
export interface ListarReprovacoesResponse {
  reprovacoes: AvaliacaoFinal[];
  total: number;
}

export interface EventosEstudanteResponse {
  codigo_estudante: string;
  nome: string;
  eventos: Evento[];
  total: number;
  message: string;
}

export interface VerificarIntegridadeResponse {
  codigo_estudante: string;
  nome: string;
  integro: boolean;
  message: string;
}

/** GET /academia/ano-letivo */
export interface AnoLetivoAcademiaResponse {
  ano_letivo: string;
  tipo?: 'escola' | 'superior';
  ativado_em?: string;
}

/**
 * @deprecated Substituído por AnoLetivoAcademiaResponse.
 */
export interface AnoLetivoResponse {
  ano_letivo: string;
}

export interface DefinirAnoLetivoResponse {
  message: string;
  ano_letivo: string;
  tipo?: string;
}

export interface AlterarCursoResponse {
  message: string;
  codigo_estudante: string;
  tipo_ensino: string;
  curso_id: string;
  curso_nome: string;
}

export interface AtualizarStatusResponse {
  message: string;
  estudante: string;
  novo_status: string;
}

export interface DeletarTurmaResponse {
  message: string;
  codigo_turma: string;
  auditavel: true;
}

export interface DeletarCursoResponse {
  message: string;
  curso_id: string;
  nome: string;
  materias_deletadas: string[];
  turmas_deletadas: string[];
  auditavel: true;
}

export interface ListarAdminsResponse {
  admins: AdminDetalhado[];
  total: number;
}

export interface PrimeiroAdminResponse {
  message: string;
  admin: AdminDetalhado;
}

// ── Perfil e consultas ────────────────────────────────────────────────────────

/**
 * GET /meu-perfil
 * Retorna { tipo, academia } | { tipo, admin } | { tipo, estudante }
 *
 * NOTA: O backend retorna academia_info dentro do estudante (não academia).
 */
export interface MeuPerfilResponse {
  tipo: UserType;
  estudante?: EstudanteDetalhado & {
    /** Informações da academia vinculada ao estudante */
    academia_info?: {
      codigo: string;
      nome: string;
      tipo: AcademiaType;
    };
    /** @deprecated Use academia_info */
    academia?: {
      codigo: string;
      nome: string;
      tipo: AcademiaType;
    };
    curso_medio?: {
      id: string;
      nome: string;
      type: CursoType;
      status: string;
    };
    curso_superior?: {
      id: string;
      nome: string;
      type: CursoType;
      status: string;
    };
  };
  academia?: AcademiaDetalhada;
  admin?: AdminDetalhado;
}

export interface ConsultarEstudanteResponse {
  estudante: EstudanteDetalhado & {
    academia?: {
      codigo: string;
      nome: string;
      tipo: AcademiaType;
    };
    curso_medio?: {
      id: string;
      nome: string;
      type: CursoType;
      status: string;
    };
    curso_superior?: {
      id: string;
      nome: string;
      type: CursoType;
      status: string;
    };
  };
}

export interface ConsultarAcademiaResponse {
  academia: AcademiaDetalhada & {
    motivo_desativacao?: string;
  };
}

export interface ConsultarAcademiasResponse {
  academias: AcademiaDetalhada[];
  total: number;
  tipo_usuario: UserType;
}

export interface ConsultarEstudantesResponse {
  estudantes: EstudanteDetalhado[];
  total: number;
  tipo_usuario: UserType;
  codigo_academia?: string;
  nome_academia?: string;
}

export interface BuscarUsuarioResponse {
  tipo: UserType;
  usuario: EstudanteDetalhado | AcademiaDetalhada | AdminDetalhado;
}

// ── Event Sourcing / Registros ───────────────────────────────────────────────

export interface RegistroCompleto {
  notas?: Nota[];
  total_notas?: number;
  total_notas_geral?: number;
  faltas?: Falta[];
  total_faltas?: number;
  total_faltas_geral?: number;
  estatisticas?: {
    total_estudantes: number;
    total_academias: number;
    total_notas: number;
    total_faltas: number;
  };
  limit?: number;
  offset?: number;
  filtro_tipo?: string;
}

// =====================
// PROVÍNCIAS
// =====================

export interface Provincia {
  nome: ProvinciaNome;
  codigo: ProvinciaCodigo;
}

export type ProvinciaNome =
  | 'BENGO' | 'BENGUELA' | 'BIE' | 'CABINDA' | 'CUANDO CUBANGO'
  | 'CUANZA NORTE' | 'CUANZA SUL' | 'CUBANGO' | 'CUNENE' | 'HUAMBO'
  | 'HUILA' | 'ICOLO E BENGO' | 'LUANDA' | 'LUNDA NORTE' | 'LUNDA SUL'
  | 'MALANJE' | 'MOXICO' | 'MOXICO LESTE' | 'NAMIBE' | 'UIGE' | 'ZAIRE';

export type ProvinciaCodigo =
  | 'BGO' | 'BGU' | 'BIE' | 'CAB' | 'CND' | 'CNO' | 'CUS'
  | 'CBG' | 'CNN' | 'HUA' | 'HUI' | 'IBG' | 'LUA'
  | 'LNO' | 'LSU' | 'MAL' | 'MOX' | 'MXL' | 'NAM'
  | 'UIG' | 'ZAI';

export const Provincias: Provincia[] = [
  { nome: 'BENGO',          codigo: 'BGO' },
  { nome: 'BENGUELA',       codigo: 'BGU' },
  { nome: 'BIE',            codigo: 'BIE' },
  { nome: 'CABINDA',        codigo: 'CAB' },
  { nome: 'CUANDO CUBANGO', codigo: 'CND' },
  { nome: 'CUANZA NORTE',   codigo: 'CNO' },
  { nome: 'CUANZA SUL',     codigo: 'CUS' },
  { nome: 'CUBANGO',        codigo: 'CBG' },
  { nome: 'CUNENE',         codigo: 'CNN' },
  { nome: 'HUAMBO',         codigo: 'HUA' },
  { nome: 'HUILA',          codigo: 'HUI' },
  { nome: 'ICOLO E BENGO',  codigo: 'IBG' },
  { nome: 'LUANDA',         codigo: 'LUA' },
  { nome: 'LUNDA NORTE',    codigo: 'LNO' },
  { nome: 'LUNDA SUL',      codigo: 'LSU' },
  { nome: 'MALANJE',        codigo: 'MAL' },
  { nome: 'MOXICO',         codigo: 'MOX' },
  { nome: 'MOXICO LESTE',   codigo: 'MXL' },
  { nome: 'NAMIBE',         codigo: 'NAM' },
  { nome: 'UIGE',           codigo: 'UIG' },
  { nome: 'ZAIRE',          codigo: 'ZAI' },
];

// =====================
// HELPERS
// =====================

export function formatAnoLetivo(valor: string): string {
  return valor.replace('_', '/');
}

export function gerarOpcoesAnoLetivo(): { valor: string; label: string }[] {
  const anoAtual = new Date().getFullYear();
  return [
    { valor: `${anoAtual - 1}_${anoAtual}`, label: `${anoAtual - 1}/${anoAtual}` },
    { valor: `${anoAtual}_${anoAtual + 1}`, label: `${anoAtual}/${anoAtual + 1}` },
  ];
}

export function isAnoFundamentalValido(ano: string): ano is AnoFundamental {
  return /^[1-9]_ano_fundamental$/.test(ano);
}

export function isAnoMedioValido(ano: string): ano is AnoMedio {
  return /^[1-9]\d*_ano_medio$/.test(ano);
}

export function isAnoSuperiorValido(ano: string): ano is AnoSuperior {
  return /^[1-9]\d*_ano_superior$/.test(ano);
}

export function isSemestreValido(periodo: string): boolean {
  return /^[1-9]\d*_semestre$/.test(periodo);
}

export function gerarAnosFundamentais(): AnoFundamental[] {
  return Array.from({ length: 9 }, (_, i) => `${i + 1}_ano_fundamental` as AnoFundamental);
}

export function formatAnoAcademico(ano: string): string {
  const match = ano.match(/^(\d+)_ano_(fundamental|medio|superior)$/);
  if (!match) return ano;
  const [, n, tipo] = match;
  const label = { fundamental: 'Fundamental', medio: 'Médio', superior: 'Superior' }[tipo] ?? tipo;
  return `${n}.º Ano (${label})`;
}

export function formatDataNascimento(iso: string): string {
  if (!iso) return '';
  const [year, month, day] = iso.split('-');
  return `${day}/${month}/${year}`;
}

export function dateToIso(date: Date): string {
  return date.toISOString().split('T')[0];
}