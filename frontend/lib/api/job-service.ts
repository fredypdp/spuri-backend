// src/lib/api/job-service.ts
import { api, tokenStorage } from './client';

// =====================
// Tipos de job
// =====================

export type JobStatus = 'pending' | 'processing' | 'done' | 'failed';

export type JobType =
  | 'register_academia_batch'
  | 'ativar_academia_batch'
  | 'desativar_academia_batch'
  | 'register_estudante_batch'
  | 'registrar_nota_batch'
  | 'atualizar_nota_batch'
  | 'deletar_nota_batch'
  | 'registrar_faltas_batch'
  | 'atualizar_falta_batch'
  | 'deletar_falta_batch'
  | 'registrar_avaliacao_final_batch'
  | 'atualizar_status_escolar_batch'
  | 'criar_curso_batch'
  | 'criar_materia_batch'
  | 'criar_turma_batch'
  | 'adicionar_estudante_batch';

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
  erro?: string;
}

export interface JobDetail extends JobSummary {
  results: JobItemResult[];
}

export interface AsyncBatchResponse {
  message: string;
  job_id: string;
  total_items: number;
  status: JobStatus;
  poll_url: string;
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

  /** Obtém o job completo com resultados por item. */
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

/**
 * Faz polling de um job até completar ou atingir timeout.
 *
 * Usa backoff exponencial suave para não sobrecarregar o servidor:
 * 1.5s → 2s → 3s → 4s → 6s → 8s (máx)
 *
 * @returns JobDetail quando concluído
 * @throws Error em timeout ou erro fatal
 *
 * @example
 * const detail = await pollJob('uuid-do-job', {
 *   onProgress: (s) => setProgress(s.progress),
 *   onComplete: (d) => console.log('concluído', d),
 * });
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
        // Buscar os resultados completos
        const detail = await jobApiService.getDetail(jobId);
        const jobDetail: JobDetail = { ...detail.job, results: detail.results };
        onComplete?.(jobDetail);
        return jobDetail;
      }

      // Backoff suave: aumentar intervalo gradualmente enquanto o job está em andamento
      currentInterval = Math.min(currentInterval * 1.3, maxIntervalMs);

    } catch (err) {
      consecutiveErrors++;
      const error = err instanceof Error ? err : new Error(String(err));
      onError?.(error);

      // Tolerar até 5 erros consecutivos antes de desistir (resiliência a instabilidade de rede)
      if (consecutiveErrors >= 5) {
        throw new Error(`Polling abortado após ${consecutiveErrors} erros consecutivos: ${error.message}`);
      }

      // Espera extra após erro
      currentInterval = Math.min(currentInterval * 2, maxIntervalMs);
    }
  }

  throw new Error(`Timeout: job ${jobId} não concluiu em ${timeoutMs / 1000}s`);
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}