// src/lib/api/job-service.ts
import { api, getApiBaseUrl, tokenStorage } from './client';

// =====================
// Tipos de job
// =====================

export type JobStatus = 'pending' | 'processing' | 'done' | 'failed';

export type JobType =
  // Academia — estudantes
  | 'register_estudante_batch'
  // Academia — notas
  | 'registrar_nota_batch'
  | 'atualizar_nota_batch'
  | 'deletar_nota_batch'
  // Academia — faltas
  | 'registrar_faltas_batch'
  | 'atualizar_falta_batch'
  | 'deletar_falta_batch'
  // Academia — avaliações
  | 'registrar_avaliacao_final_batch'
  // Academia — status escolar
  | 'atualizar_status_escolar_batch'
  // Academia — cursos
  | 'criar_curso_batch'
  | 'ativar_curso_batch'
  | 'desativar_curso_batch'
  | 'atualizar_curso_batch'
  | 'deletar_curso_batch'
  // Academia — matérias
  | 'criar_materia_batch'
  | 'ativar_materia_batch'
  | 'desativar_materia_batch'
  | 'definir_periodo_materia_batch'
  | 'atualizar_materia_batch'
  | 'deletar_materia_batch'
  // Academia — turmas
  | 'criar_turma_batch'
  | 'ativar_turma_batch'
  | 'desativar_turma_batch'
  | 'atualizar_turma_batch'
  | 'deletar_turma_batch'
  // Academia — turmas / estudantes
  | 'adicionar_estudante_batch'
  | 'remover_estudante_turma_batch'
  // Academia — dados / categorias de nota
  | 'atualizar_academia_batch'
  | 'criar_categoria_nota_batch'
  | 'deletar_categoria_nota_batch'
  // Admin — academias
  | 'register_academia_batch'
  | 'ativar_academia_batch'
  | 'desativar_academia_batch'
  // Admin — admins
  | 'ativar_admin_batch'
  | 'desativar_admin_batch'
  // Admin — rebuild de projeção
  | 'rebuild_projection';

export interface JobSummary {
  id: string;
  type: JobType;
  status: JobStatus;
  progress: number;       // 0..100
  total_items: number;
  done_items: number;
  fail_items: number;
  error?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface JobItemResult {
  index: number;
  sucesso: boolean;
  dados?: unknown;
  /** Motivo da falha — pode vir como `erro`, `error` ou `message` dependendo da versão do backend */
  erro?: string;
  error?: string;
  message?: string;
  /** Payload original do item para replay */
  payload?: unknown;
}

/**
 * JobDetail é retornado pelo endpoint `GET /jobs/:id?results=true`.
 * O backend envolve em `{ job, results }`.
 * Internamente normalizamos para uma estrutura plana com `results`.
 */
export interface JobDetail extends JobSummary {
  results: JobItemResult[];
}

/**
 * Resposta padrão de todas as rotas batch assíncronas (202 Accepted).
 * Inclui `sse_url` desde a versão 1.0.9 do backend.
 */
export interface AsyncBatchResponse {
  message: string;
  job_id: string;
  total_items: number;
  status: JobStatus;
  /** URL de polling: GET /jobs/:id */
  poll_url: string;
  /** URL SSE para acompanhamento em tempo real: GET /jobs/stream */
  sse_url: string;
}

// =====================
// Helpers internos
// =====================

/**
 * Extrai a melhor mensagem de erro disponível num item de resultado,
 * respeitando os diferentes campos que o backend pode retornar.
 */
export function resolveJobItemError(item: JobItemResult): string | undefined {
  const raw = item.erro ?? item.error ?? item.message;
  if (!raw) return undefined;
  if (typeof raw === 'string') return raw;
  try { return JSON.stringify(raw); } catch { return String(raw); }
}

// =====================
// API calls
// =====================

export const jobApiService = {
  /** Obtém o summary de um job (leve, para polling). */
  getStatus: (jobId: string, token?: string): Promise<JobSummary> =>
    api.get<JobSummary>(`/jobs/${jobId}`, {
      token: token || tokenStorage.get() || undefined,
    }),

  /**
   * Obtém o job completo com resultados por item.
   * A API retorna `{ job: JobSummary, results: JobItemResult[] }`.
   * Retornamos a estrutura original para não quebrar code que depende de `.job`.
   */
  getDetail: (jobId: string, token?: string): Promise<{ job: JobSummary; results: JobItemResult[] }> =>
    api.get<{ job: JobSummary; results: JobItemResult[] }>(`/jobs/${jobId}?results=true`, {
      token: token || tokenStorage.get() || undefined,
    }),

  /** Lista os jobs recentes do usuário autenticado. */
  list: (token?: string): Promise<{ jobs: JobSummary[]; total: number }> =>
    api.get<{ jobs: JobSummary[]; total: number }>('/jobs', {
      token: token || tokenStorage.get() || undefined,
    }),
};

// =====================
// Polling helper
// =====================

export interface PollOptions {
  /** Intervalo inicial entre polls em ms (padrão: 1500) */
  intervalMs?: number;
  /** Intervalo máximo após backoff em ms (padrão: 8000) */
  maxIntervalMs?: number;
  /** Timeout total em ms (padrão: 10 minutos) */
  timeoutMs?: number;
  /** Callback chamado a cada atualização de progresso */
  onProgress?: (summary: JobSummary) => void;
  /** Callback chamado quando o job completa (done ou failed) */
  onComplete?: (detail: JobDetail) => void;
  /** Callback chamado em erro de rede durante o polling */
  onError?: (error: Error) => void;
}

export type JobEventType = 'job_enqueued' | 'job_progress' | 'job_done' | 'job_failed';

export interface JobStreamEvent {
  type: JobEventType;
  job_id: string;
  job_type?: JobType | string;
  status?: JobStatus;
  progress?: number;
  done_items?: number;
  fail_items?: number;
  total_items?: number;
  error?: string;
  message?: string;
}

export interface JobStreamOptions {
  token?: string;
  onEvent: (event: JobStreamEvent) => void;
  onError?: (error: Error) => void;
  signal?: AbortSignal;
}

function parseSseData(data: string): JobStreamEvent | null {
  if (!data) return null;
  try {
    return JSON.parse(data) as JobStreamEvent;
  } catch {
    return null;
  }
}

/**
 * Faz polling de um job até completar ou atingir timeout.
 *
 * Usa backoff exponencial suave para não sobrecarregar o servidor:
 * 1.5s → 2s → 3s → 4s → 6s → 8s (máx)
 *
 * @returns JobDetail quando concluído
 * @throws Error em timeout ou erro fatal
 */
export async function pollJob(jobId: string, options: PollOptions = {}): Promise<JobDetail> {
  const {
    intervalMs = 1500,
    maxIntervalMs = 8000,
    timeoutMs = 10 * 60 * 1000,
    onProgress,
    onComplete,
    onError,
  } = options;

  const deadline = Date.now() + timeoutMs;
  let currentInterval = intervalMs;
  let consecutiveErrors = 0;

  while (Date.now() < deadline) {
    await sleep(currentInterval);

    try {
      const summary = await jobApiService.getStatus(jobId);
      consecutiveErrors = 0;

      onProgress?.(summary);

      if (summary.status === 'done' || summary.status === 'failed') {
        const detailResponse = await jobApiService.getDetail(jobId);
        const normalizedResults = (detailResponse.results ?? []).map((item) => ({
          ...item,
          erro: resolveJobItemError(item),
        }));
        const jobDetail: JobDetail = {
          ...detailResponse.job,
          results: normalizedResults,
        };
        onComplete?.(jobDetail);
        return jobDetail;
      }

      currentInterval = Math.min(currentInterval * 1.3, maxIntervalMs);

    } catch (err) {
      consecutiveErrors++;
      const error = err instanceof Error ? err : new Error(String(err));
      onError?.(error);

      if (consecutiveErrors >= 5) {
        throw new Error(`Polling abortado após ${consecutiveErrors} erros consecutivos: ${error.message}`);
      }

      currentInterval = Math.min(currentInterval * 2, maxIntervalMs);
    }
  }

  throw new Error(`Timeout: job ${jobId} não concluiu em ${timeoutMs / 1000}s`);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

export async function subscribeToJobStream({
  token,
  onEvent,
  onError,
  signal,
}: JobStreamOptions): Promise<void> {
  const authToken = token || tokenStorage.get() || undefined;
  const baseUrl = getApiBaseUrl();

  if (!baseUrl) {
    onError?.(new Error('API_URL não está configurada'));
    return;
  }

  if (!authToken) {
    onError?.(new Error('Token não encontrado para abrir stream de jobs'));
    return;
  }

  let response: Response;
  try {
    response = await fetch(`${baseUrl}/jobs/stream`, {
      method: 'GET',
      headers: {
        Authorization: `Bearer ${authToken}`,
        Accept: 'text/event-stream',
        'Cache-Control': 'no-cache',
      },
      signal,
    });
  } catch (err) {
    if (signal?.aborted) return;
    onError?.(err instanceof Error ? err : new Error(String(err)));
    return;
  }

  if (!response.ok || !response.body) {
    const detail = `${response.status} ${response.statusText}`.trim();
    onError?.(new Error(`Falha ao abrir stream de jobs: ${detail}`));
    return;
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let eventType = '';
  let eventData = '';

  const flushEvent = () => {
    const parsed = parseSseData(eventData.trim());
    if (!parsed) {
      eventType = '';
      eventData = '';
      return;
    }
    onEvent({
      ...parsed,
      type: (parsed.type || eventType || 'job_progress') as JobEventType,
    });
    eventType = '';
    eventData = '';
  };

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() ?? '';

      for (const rawLine of lines) {
        const line = rawLine.trimEnd();

        if (!line) {
          flushEvent();
          continue;
        }

        if (line.startsWith(':')) {
          continue; // heartbeat ping — ignorar
        }

        if (line.startsWith('event:')) {
          eventType = line.slice(6).trim();
          continue;
        }

        if (line.startsWith('data:')) {
          const chunk = line.slice(5).trim();
          eventData = eventData ? `${eventData}\n${chunk}` : chunk;
        }
      }
    }
  } catch (err) {
    if (signal?.aborted) return;
    const error = err instanceof Error ? err : new Error(String(err));
    onError?.(error);
  } finally {
    reader.releaseLock();
  }
}