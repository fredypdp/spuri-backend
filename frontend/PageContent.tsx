"use client";
import { useState, useCallback, useRef, useEffect } from "react";
import { jobApiService, pollJob, tokenStorage } from "@/lib/api";
import { resolveJobItemError } from "@/lib/api/job-service";
import { getCookie } from "@/lib/utils/cookies";
import type { MeuPerfilResponse } from "@/types/api";

// ─── Types ────────────────────────────────────────────────────────────────────

type LogLevel = "ok" | "err" | "warn" | "info" | "step" | "dim";
interface LogEntry { ts: string; level: LogLevel; msg: string; }

interface AcademiaInfo {
  codigo: string;
  token: string;
  tipo: "escola" | "superior";
  nivel?: "fundamental" | "medio" | "misto";
  anos_academicos?: string[];
  ano_letivo?: string;
}

interface Materia {
  id: string;
  nome: string;
  type: string;
  anos_academicos: string[];
  periodo?: string;
  curso_id?: string;
}

interface Estudante {
  codigo_estudante: string;
  nome: string;
  ano_escolar?: string;
  ano_escolar_medio?: string;
  ano_superior?: string;
  total_notas?: number;
  total_faltas?: number;
}
interface Turma { id?: string; codigo_turma: string; nivel: string; estudantes: string[]; curso_id?: string; status?: string; }
interface Curso { id: string; nome: string; type: string; anos_academicos: string[]; periodos?: string[]; status?: string; }

// ─── Constants ─────────────────────────────────────────────────────────────────

const NOMES_M = ["João","António","Manuel","Francisco","Domingos","Pedro","Paulo","Carlos","Luís","Miguel","Filipe","Rui","Hélder","Faustino","Simão","Narciso","Mário","Sérgio","Ezequiel","Armindo"];
const NOMES_F = ["Maria","Ana","Sofia","Isabel","Filomena","Rosa","Conceição","Graça","Fernanda","Lurdes","Beatriz","Carla","Diana","Elisa","Fátima","Glória","Helena","Inês","Joana","Kátia"];
const SOBRENOMES = ["Silva","Santos","Costa","Ferreira","Oliveira","Neto","Lopes","Fernandes","Gonçalves","Rodrigues","Monteiro","Cardoso","Marques","Correia","Mendes","Kiala","Nzinga","Mbemba","Lukamba","Tchipilica"];

const CURSOS_MEDIO = [
  { nome: "Ciências e Tecnologia", anos: ["1_ano_medio","2_ano_medio","3_ano_medio"] },
  { nome: "Letras e Ciências Humanas", anos: ["1_ano_medio","2_ano_medio","3_ano_medio"] },
  { nome: "Económico-Jurídico", anos: ["1_ano_medio","2_ano_medio","3_ano_medio"] },
  { nome: "Informática e Gestão", anos: ["1_ano_medio","2_ano_medio","3_ano_medio"] },
];
const CURSOS_SUPERIOR = [
  { nome: "Engenharia Informática", anos: ["1_ano_superior","2_ano_superior","3_ano_superior","4_ano_superior","5_ano_superior"], periodos: ["1_semestre","2_semestre"] },
  { nome: "Medicina", anos: ["1_ano_superior","2_ano_superior","3_ano_superior","4_ano_superior","5_ano_superior","6_ano_superior"], periodos: ["1_semestre","2_semestre"] },
  { nome: "Direito", anos: ["1_ano_superior","2_ano_superior","3_ano_superior","4_ano_superior"], periodos: ["1_semestre","2_semestre"] },
  { nome: "Gestão de Empresas", anos: ["1_ano_superior","2_ano_superior","3_ano_superior","4_ano_superior"], periodos: ["1_semestre","2_semestre"] },
  { nome: "Psicologia", anos: ["1_ano_superior","2_ano_superior","3_ano_superior","4_ano_superior","5_ano_superior"], periodos: ["1_semestre","2_semestre"] },
];

const MATERIAS_FUND = ["Língua Portuguesa","Matemática","Estudo do Meio","Educação Visual","Educação Física","Música","Inglês","Ciências Naturais","História de Angola","Formação Cívica"];
const MATERIAS_MEDIO = ["Língua Portuguesa","Matemática","Física","Química","Biologia","História","Geografia","Filosofia","Inglês","Educação Física","Informática","Economia"];
const MATERIAS_SUPERIOR = ["Álgebra Linear","Cálculo I","Programação","Estruturas de Dados","Redes","Sistemas Operativos","Base de Dados","Engenharia de Software","Inteligência Artificial","Segurança Informática"];

const TURNOS = ["manha","tarde","noite"] as const;
const DATAS_FALTA = ["2025-03-10","2025-04-07","2025-05-05","2025-06-02","2025-07-07","2025-09-01","2025-10-06","2025-11-03"];

// ─── Helpers ─────────────────────────────────────────────────────────────────

const rnd = (a: number, b: number) => Math.floor(Math.random() * (b - a + 1)) + a;
const pick = <T,>(arr: T[]): T => arr[Math.floor(Math.random() * arr.length)];
const pickN = <T,>(arr: T[], n: number): T[] => [...arr].sort(() => 0.5 - Math.random()).slice(0, n);
const sleep = (ms: number) => new Promise<void>(r => setTimeout(r, ms));
const gerarNome = () => {
  const m = Math.random() < 0.51;
  return { nome: `${pick(m ? NOMES_M : NOMES_F)} ${pick(SOBRENOMES)} ${pick(SOBRENOMES)}`, genero: m ? "masculino" as const : "feminino" as const };
};
const gerarDataNasc = (minAge = 8, maxAge = 25) => {
  const dias = rnd(minAge, maxAge) * 365 + rnd(0, 364);
  return new Date(Date.now() - dias * 86400000).toISOString();
};

function tiposMateriaValidos(academia: AcademiaInfo): { value: "fundamental"|"medio"|"superior"; label: string }[] {
  if (academia.tipo === "superior") {
    return [{ value: "superior", label: "Superior" }];
  }
  if (academia.nivel === "fundamental") {
    return [{ value: "fundamental", label: "Fundamental" }];
  }
  if (academia.nivel === "medio") {
    return [{ value: "medio", label: "Médio" }];
  }
  if (academia.nivel === "misto") {
    return [
      { value: "fundamental", label: "Fundamental" },
      { value: "medio", label: "Médio" },
    ];
  }
  return [{ value: "fundamental", label: "Fundamental" }];
}

function tiposCursoValidos(academia: AcademiaInfo): { value: "medio"|"superior"; label: string }[] {
  if (academia.tipo === "superior") {
    return [{ value: "superior", label: "Superior" }];
  }
  if (academia.nivel === "medio" || academia.nivel === "misto") {
    return [{ value: "medio", label: "Médio" }];
  }
  return [];
}

function tiposEnsinoDisponiveis(academia: AcademiaInfo, cursos: Curso[]): { value: string; label: string }[] {
  const tipos: { value: string; label: string }[] = [];
  if (academia.tipo === "escola") {
    if (academia.nivel === "fundamental" || academia.nivel === "misto") {
      tipos.push({ value: "fundamental", label: "Fundamental" });
    }
    const temCursoMedio = cursos.some(c => c.type === "medio");
    if ((academia.nivel === "medio" || academia.nivel === "misto") && temCursoMedio) {
      tipos.push({ value: "medio", label: "Médio" });
    }
  } else if (academia.tipo === "superior") {
    const temCursoSuperior = cursos.some(c => c.type === "superior");
    if (temCursoSuperior) {
      tipos.push({ value: "superior", label: "Superior" });
    }
  }
  return tipos;
}

type EscolaMode = "fundamental" | "medio" | "superior" | "misto";

function getEscolaMode(academia: AcademiaInfo): EscolaMode {
  if (academia.tipo === "superior") return "superior";
  if (academia.nivel === "medio") return "medio";
  if (academia.nivel === "misto") return "misto";
  return "fundamental";
}

// ─── Main Component ───────────────────────────────────────────────────────────

export default function SeedTestPage() {
  const [currentUser, setCurrentUser] = useState<MeuPerfilResponse | null>(null);
  const [academia, setAcademia] = useState<AcademiaInfo | null>(null);
  const [authError, setAuthError] = useState<string>("");

  const [cursos, setCursos] = useState<Curso[]>([]);
  const [materias, setMaterias] = useState<Materia[]>([]);
  const [turmas, setTurmas] = useState<Turma[]>([]);
  const [estudantes, setEstudantes] = useState<Estudante[]>([]);

  const [cursoConfig, setCursoConfig] = useState({ tipo: "medio" as "medio"|"superior", qtd: 2 });
  const [materiaConfig, setMateriaConfig] = useState({ tipo: "fundamental" as "fundamental"|"medio"|"superior", qtd: 5, cursoId: "" });

  const [turmaConfig, setTurmaConfig] = useState({ qtd: 3, turno: "random" as string, nivel: "random", cursoId: "random" });
  const [vincularConfig, setVincularConfig] = useState({ turmaCodigo: "random" });
  const [notaConfig, setNotaConfig] = useState({ qtdEstudantes: 0, periodo: "random" });
  const [faltaConfig, setFaltaConfig] = useState({ qtdEstudantes: 0 });
  const [avalConfig, setAvalConfig] = useState({ tipoEnsino: "fundamental" as string, aprovPct: 70 });

  const [estudanteConfig, setEstudanteConfig] = useState({
    qtd: 20,
    anoFundamental: "random",
    statusFundamental: "em_andamento",
    anoMedio: "random",
    statusMedio: "em_andamento",
    cursoMedioId: "random",
    anoSuperior: "random",
    statusSuperior: "em_andamento",
    cursoSuperiorId: "random",
    modoPrincipal: "fundamental" as "fundamental" | "medio",
    pctFundamental: 60,
  });

  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [running, setRunning] = useState(false);
  const logsEndRef = useRef<HTMLDivElement>(null);
  const cancelRef = useRef(false);

  const addLog = useCallback((msg: string, level: LogLevel = "info") => {
    const ts = new Date().toLocaleTimeString("pt-PT", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
    setLogs(prev => [...prev, { ts, level, msg }].slice(-800));
    setTimeout(() => logsEndRef.current?.scrollIntoView({ behavior: "smooth" }), 30);
  }, []);

  useEffect(() => {
    const userCookie = getCookie("user");
    const token = tokenStorage.get();
    if (!userCookie || !token) { setAuthError("Sem sessão ativa. Faça login como academia para usar esta página."); return; }
    try {
      const parsed: MeuPerfilResponse = JSON.parse(userCookie);
      if (parsed.tipo !== "academia") { setAuthError("Esta página é exclusiva para academias."); return; }
      if (!parsed.academia) { setAuthError("Dados da academia não encontrados. Faça login novamente."); return; }
      setCurrentUser(parsed);
      const ac = parsed.academia;
      const acInfo: AcademiaInfo = {
        codigo: ac.codigo_academia,
        token,
        tipo: ac.type,
        nivel: ac.nivel_escolar,
        anos_academicos: ac.anos_academicos || [],
        ano_letivo: ac.ano_letivo,
      };
      setAcademia(acInfo);

      const tiposCurso = tiposCursoValidos(acInfo);
      if (tiposCurso.length > 0) setCursoConfig(p => ({ ...p, tipo: tiposCurso[0].value }));
      const tiposMateria = tiposMateriaValidos(acInfo);
      if (tiposMateria.length > 0) setMateriaConfig(p => ({ ...p, tipo: tiposMateria[0].value }));

      if (acInfo.nivel === "misto") {
        setEstudanteConfig(p => ({ ...p, modoPrincipal: "fundamental" }));
      }
    } catch { setAuthError("Erro ao ler dados da sessão. Faça login novamente."); }
  }, []);

  useEffect(() => {
    if (!academia) return;
    const tipos = tiposEnsinoDisponiveis(academia, cursos);
    if (tipos.length > 0) {
      setAvalConfig(p => ({ ...p, tipoEnsino: tipos[0].value }));
    }
  }, [academia, cursos]);

  useEffect(() => {
    if (estudantes.length > 0) {
      setNotaConfig(p => ({ ...p, qtdEstudantes: p.qtdEstudantes === 0 ? estudantes.length : p.qtdEstudantes }));
      setFaltaConfig(p => ({ ...p, qtdEstudantes: p.qtdEstudantes === 0 ? estudantes.length : p.qtdEstudantes }));
    }
  }, [estudantes.length]);

  useEffect(() => { if (academia) refreshData(academia); }, [academia?.codigo]); // eslint-disable-line react-hooks/exhaustive-deps

  const apiUrl = () => {
    const url = process.env.NEXT_PUBLIC_API_URL || "";
    if (url && !url.startsWith("http://") && !url.startsWith("https://")) return `https://${url}`;
    return url;
  };

  const callApi = async (method: string, path: string, body: unknown, tok?: string) => {
    const url = apiUrl() + path;
    const token = tok || academia?.token || tokenStorage.get() || "";
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (token) headers["Authorization"] = `Bearer ${token}`;
      const r = await fetch(url, { method, headers, body: body != null ? JSON.stringify(body) : undefined });
      const data = await r.json().catch(() => ({}));
      return { ok: r.status >= 200 && r.status < 300, status: r.status, data };
    } catch (e) {
      return { ok: false, status: 0, data: { error: String(e) } };
    }
  };

  const refreshData = async (ac?: AcademiaInfo) => {
    const acad = ac || academia;
    if (!acad?.token) return;
    const tok = acad.token;
    const [rCursos, rMaterias, rTurmas, rEstudantes] = await Promise.all([
      callApi("GET", "/academia/cursos", undefined, tok),
      callApi("GET", "/academia/materias", undefined, tok),
      callApi("GET", "/academia/turmas", undefined, tok),
      callApi("GET", "/estudantes", undefined, tok),
    ]);
    const cursosData: Curso[] = (rCursos.data as any)?.cursos || [];
    const materiasData: Materia[] = (rMaterias.data as any)?.materias?.filter((m: any) => m.status === "ativo") || [];
    const turmasData: Turma[] = (rTurmas.data as any)?.turmas || [];
    const estudantesData: Estudante[] = (rEstudantes.data as any)?.estudantes || [];
    setCursos(cursosData);
    setMaterias(materiasData);
    setTurmas(turmasData);
    setEstudantes(estudantesData);
    addLog(`Dados atualizados: ${cursosData.length} cursos, ${materiasData.length} matérias ativas, ${turmasData.length} turmas, ${estudantesData.length} estudantes`, "dim");
  };

  const acompanharJob = async (jobId: string, titulo: string) => {
    const detail = await pollJob(jobId, {
      timeoutMs: 5 * 60 * 1000,
      onProgress: (summary) => {
        addLog(`  📊 Progresso: ${summary.progress ?? 0}% (${summary.done_items ?? 0}/${summary.total_items ?? 0})`, "dim");
      },
    });

    const detailResponse = await jobApiService.getDetail(jobId, academia?.token);
    const failures = (detailResponse.results ?? []).filter((item) => !item.sucesso);

    if (failures.length > 0) {
      addLog(`  ⚠ ${titulo}: ${failures.length} item(ns) com falha`, "warn");
      failures.slice(0, 8).forEach((f, i) => {
        const payloadAny = f.payload as any;
        const label =
          payloadAny?.codigo_estudante ||
          payloadAny?.codigo_turma ||
          payloadAny?.codigo ||
          payloadAny?.nome ||
          `item #${(f.index ?? i) + 1}`;
        const motivo = resolveJobItemError(f) || detail.error || 'Falha sem detalhe retornado';
        addLog(`    • ${label}: ${motivo}`, "warn");
      });
      if (failures.length > 8) addLog(`    • ...e mais ${failures.length - 8} falha(s)`, "dim");
    }

    if (detail.status === "failed" && detail.error) {
      addLog(`  ✗ ${titulo}: ${detail.error}`, "err");
    }

    return { ok: detail.done_items, err: detail.fail_items, total: detail.total_items };
  };

  const withLoading = async (fn: () => Promise<void>) => {
    setRunning(true);
    cancelRef.current = false;
    try { await fn(); } finally { setRunning(false); }
  };

  // ─── Gerar Cursos ─────────────────────────────────────────────────────────────
  const gerarCursos = async () => {
    if (!academia) return;
    const tiposValidos = tiposCursoValidos(academia);
    if (tiposValidos.length === 0) {
      addLog("  ✗ Esta academia (fundamental) não suporta cursos", "err");
      return;
    }
    const { tipo, qtd } = cursoConfig;
    if (!tiposValidos.find(t => t.value === tipo)) {
      addLog(`  ✗ Tipo de curso "${tipo}" não é válido para esta academia`, "err");
      return;
    }
    addLog(`Gerando ${qtd} curso(s) do tipo ${tipo}...`, "step");
    const templates = tipo === "medio" ? CURSOS_MEDIO : CURSOS_SUPERIOR;
    const picked = pickN(templates, Math.min(qtd, templates.length));
    for (const t of picked) {
      if (cancelRef.current) break;
      const payload: any = {
        nome: t.nome,
        type: tipo,
        anos_academicos: t.anos,
      };
      if (tipo === "superior") {
        payload.periodos = (t as any).periodos;
      }
      const { ok, data } = await callApi("POST", "/academia/curso", payload, academia.token);
      if (ok) {
        const id = (data as any).data?.id;
        addLog(`  ✓ Curso "${t.nome}" criado`, "ok");
        if (id) {
          await sleep(500);
          const { ok: okA } = await callApi("PUT", `/academia/curso/${id}/ativar`, {}, academia.token);
          if (okA) addLog(`    ✓ Curso ativado`, "dim");
        }
      } else {
        const errMsg = (data as any)?.message || (data as any)?.error || 'Erro desconhecido';
        addLog(`  ✗ "${t.nome}": ${errMsg}`, "warn");
      }
      await sleep(300);
    }
    await sleep(3000);
    await refreshData();
    addLog("Cursos gerados ✓", "ok");
  };

  // ─── Gerar Matérias ───────────────────────────────────────────────────────────
  const gerarMaterias = async () => {
    if (!academia) return;
    const tiposValidos = tiposMateriaValidos(academia);
    const { tipo, qtd, cursoId } = materiaConfig;

    if (!tiposValidos.find(t => t.value === tipo)) {
      addLog(`  ✗ Tipo de matéria "${tipo}" não é válido para esta academia (${academia.tipo}/${academia.nivel})`, "err");
      return;
    }

    if (tipo === "medio" || tipo === "superior") {
      const cursoDisponivel = cursoId && cursoId !== "auto"
        ? cursos.find(c => c.id === cursoId && c.type === tipo && c.status === "ativo")
        : cursos.find(c => c.type === tipo && c.status === "ativo");
      if (!cursoDisponivel) {
        addLog(`  ✗ Nenhum curso "${tipo}" ativo disponível — crie e ative cursos primeiro`, "err");
        return;
      }
    }

    addLog(`Gerando ${qtd} matéria(s) do tipo ${tipo}...`, "step");

    const pool = tipo === "fundamental" ? MATERIAS_FUND : tipo === "medio" ? MATERIAS_MEDIO : MATERIAS_SUPERIOR;
    const cursoAlvo = (tipo === "medio" || tipo === "superior")
      ? (cursoId && cursoId !== "auto"
          ? cursos.find(c => c.id === cursoId && c.type === tipo && c.status === "ativo")
          : cursos.find(c => c.type === tipo && c.status === "ativo"))
      : null;

    let anosDisponiveis: string[] = [];
    if (tipo === "fundamental") {
      anosDisponiveis = (academia.anos_academicos || []).filter(a => a.includes("fundamental"));
      if (anosDisponiveis.length === 0) {
        addLog("  ✗ Academia não tem anos fundamentais configurados", "err");
        return;
      }
    } else if (cursoAlvo) {
      anosDisponiveis = cursoAlvo.anos_academicos || [];
    }

    if ((tipo === "medio" || tipo === "superior") && anosDisponiveis.length === 0) {
      addLog(`  ✗ Curso "${cursoAlvo?.nome}" não tem anos académicos configurados`, "err");
      return;
    }

    const materiasExistentesNomes = materias
      .filter(m => m.type === tipo && (cursoAlvo ? m.curso_id === cursoAlvo.id : true))
      .map(m => m.nome.toLowerCase());
    const poolFiltrado = pool.filter(n => !materiasExistentesNomes.includes(n.toLowerCase()));
    if (poolFiltrado.length === 0) {
      addLog("  ✗ Todas as matérias do pool já existem para este tipo/curso", "warn");
      return;
    }

    const picked = pickN(poolFiltrado, Math.min(qtd, poolFiltrado.length));
    let criadas = 0;

    for (const nome of picked) {
      if (cancelRef.current) break;

      const anoSelecionado = pick(anosDisponiveis);
      const payload: any = {
        nome,
        type: tipo,
        anos_academicos: [anoSelecionado],
      };
      if (cursoAlvo) payload.curso_id = cursoAlvo.id;

      const { ok, data } = await callApi("POST", "/academia/materia", payload, academia.token);
      if (!ok) {
        const errMsg = (data as any)?.message || (data as any)?.error || 'Erro desconhecido';
        addLog(`  ✗ "${nome}": ${errMsg}`, "warn");
        await sleep(300);
        continue;
      }

      const id = (data as any).data?.id;
      criadas++;
      addLog(`  ✓ Matéria "${nome}" (${anoSelecionado}) ${cursoAlvo ? `→ ${cursoAlvo.nome}` : ""} criada`, "ok");

      if (tipo === "superior" && id && cursoAlvo?.periodos?.length) {
        await sleep(300);
        const periodo = pick(cursoAlvo.periodos);
        const { ok: okP, data: dataP } = await callApi("PUT", `/academia/materia/${id}/periodo`, { periodo }, academia.token);
        if (okP) {
          addLog(`    ✓ Período "${periodo}" definido`, "dim");
          await sleep(300);
          const { ok: okA } = await callApi("PUT", `/academia/materia/${id}/ativar`, {}, academia.token);
          if (okA) addLog(`    ✓ Matéria ativada`, "dim");
          else addLog(`    ! Falha ao ativar`, "warn");
        } else {
          const errP = (dataP as any)?.message || (dataP as any)?.error || 'Erro';
          addLog(`    ✗ Falha ao definir período: ${errP}`, "warn");
        }
      } else if (tipo !== "superior" && id) {
        await sleep(300);
        await callApi("PUT", `/academia/materia/${id}/ativar`, {}, academia.token);
      }

      await sleep(300);
    }

    await sleep(3000);
    await refreshData();
    addLog(`${criadas} matéria(s) gerada(s) ✓`, "ok");
  };

  // ─── Gerar Turmas ─────────────────────────────────────────────────────────────
  const gerarTurmas = async () => {
    if (!academia) return;
    const { qtd } = turmaConfig;
    addLog(`Gerando ${qtd} turma(s)...`, "step");
    let criadas = 0;

    const niveisDisponiveis: string[] = [];
    if (academia.tipo === "escola") {
      if (academia.nivel === "fundamental" || academia.nivel === "misto") {
        niveisDisponiveis.push(...(academia.anos_academicos?.filter(a => a.includes("fundamental")) || ["1_ano_fundamental"]));
      }
      if (academia.nivel === "medio" || academia.nivel === "misto") {
        const cursosMedio = cursos.filter(c => c.type === "medio" && c.status === "ativo");
        for (const curso of cursosMedio) niveisDisponiveis.push(...curso.anos_academicos);
      }
    } else if (academia.tipo === "superior") {
      const cursosSup = cursos.filter(c => c.type === "superior" && c.status === "ativo");
      for (const curso of cursosSup) niveisDisponiveis.push(...curso.anos_academicos);
    }

    if (niveisDisponiveis.length === 0) {
      addLog("  ✗ Nenhum nível disponível para criar turmas.", "err");
      return;
    }

    const tipoCurso = academia.tipo === "superior" ? "superior" : "medio";
    const cursosAtivos = cursos.filter(c => c.type === tipoCurso && c.status === "ativo");

    const cursoEspecifico = turmaConfig.cursoId !== "random"
      ? cursosAtivos.find(c => c.id === turmaConfig.cursoId)
      : undefined;

    if (turmaConfig.cursoId !== "random" && !cursoEspecifico) {
      addLog(`  ✗ Curso selecionado não encontrado ou inativo`, "err");
      return;
    }

    for (let i = 0; i < qtd; i++) {
      if (cancelRef.current) break;

      const nivel = turmaConfig.nivel === "random" ? pick(niveisDisponiveis) : turmaConfig.nivel;
      const turno = turmaConfig.turno === "random" ? pick([...TURNOS]) : turmaConfig.turno as typeof TURNOS[number];
      const letra = String.fromCharCode(65 + (i % 26));
      const payload: any = { codigo_turma: `T${rnd(1, 9)}${letra}${rnd(10, 99)}`, nivel, turno };

      if (nivel.includes("medio") || nivel.includes("superior")) {
        let cursoAlvo: Curso | undefined;
        if (cursoEspecifico) {
          cursoAlvo = cursoEspecifico;
        } else if (turmaConfig.cursoId === "random") {
          const cursosCompativeis = cursosAtivos.filter(c => c.anos_academicos.includes(nivel));
          if (cursosCompativeis.length > 0) {
            cursoAlvo = pick(cursosCompativeis);
          } else {
            cursoAlvo = cursosAtivos.length > 0 ? pick(cursosAtivos) : undefined;
          }
        }
        if (cursoAlvo) {
          payload.curso_id = cursoAlvo.id;
          addLog(`  · Turma ${payload.codigo_turma} → ${cursoAlvo.nome} (${nivel})`, "dim");
        }
      }

      const { ok, data } = await callApi("POST", "/academia/turma", payload, academia.token);
      if (ok) {
        criadas++;
        addLog(`  ✓ Turma ${payload.codigo_turma} (${nivel}, ${turno}) criada`, "ok");
      } else {
        const errMsg = (data as any)?.message || (data as any)?.error || 'Erro desconhecido';
        addLog(`  ✗ Turma ${payload.codigo_turma}: ${errMsg}`, "warn");
      }
      await sleep(200);
    }
    await sleep(2000);
    await refreshData();
    addLog(`${criadas} turma(s) gerada(s) ✓`, "ok");
  };

  // ─── Gerar Estudantes ─────────────────────────────────────────────────────────
  const gerarEstudantes = async () => {
    if (!academia) return;
    const cfg = estudanteConfig;
    const modo = getEscolaMode(academia);

    addLog(`Gerando ${cfg.qtd} estudante(s) via async (modo: ${modo})...`, "step");

    const cursoMedioAlvo = cfg.cursoMedioId === "random"
      ? cursos.find(c => c.type === "medio" && c.status === "ativo")
      : cursos.find(c => c.id === cfg.cursoMedioId && c.status === "ativo");

    const cursoSuperiorAlvo = cfg.cursoSuperiorId === "random"
      ? cursos.find(c => c.type === "superior" && c.status === "ativo")
      : cursos.find(c => c.id === cfg.cursoSuperiorId && c.status === "ativo");

    const anosF = (academia.anos_academicos || []).filter(a => a.includes("fundamental"));
    const anosMedio = cursoMedioAlvo?.anos_academicos || [];
    const anosSuperior = cursoSuperiorAlvo?.anos_academicos || [];

    // API /academia/estudante/register/async espera array puro de objetos
    const items: any[] = Array.from({ length: cfg.qtd }, (_, idx) => {
      const { nome, genero } = gerarNome();
      const payload: any = {
        nome,
        genero,
        data_nascimento: gerarDataNasc(),
        bilhete_identidade: `${rnd(100000000, 999999999)}LA0${rnd(10, 99)}`,
      };

      if (modo === "superior") {
        if (cursoSuperiorAlvo) {
          const ano = cfg.anoSuperior === "random"
            ? (anosSuperior.length > 0 ? pick(anosSuperior) : "1_ano_superior")
            : cfg.anoSuperior;
          payload.ano_superior = ano;
          payload.status_superior = cfg.statusSuperior;
          payload.curso_superior_id = cursoSuperiorAlvo.id;
        } else {
          addLog(`  ! Est. #${idx + 1}: nenhum curso superior ativo — criado sem vínculo de curso`, "warn");
        }
      } else if (modo === "medio") {
        if (cursoMedioAlvo && anosMedio.length > 0) {
          const ano = cfg.anoMedio === "random"
            ? pick(anosMedio)
            : cfg.anoMedio;
          payload.ano_escolar_medio = ano;
          payload.status_escolar_medio = cfg.statusMedio;
          payload.curso_medio_id = cursoMedioAlvo.id;
        } else {
          addLog(`  ! Est. #${idx + 1}: nenhum curso médio ativo — criado sem vínculo de curso`, "warn");
        }
      } else if (modo === "fundamental") {
        if (anosF.length > 0) {
          const ano = cfg.anoFundamental === "random"
            ? pick(anosF)
            : cfg.anoFundamental;
          payload.ano_escolar = ano;
          payload.status_escolar_fundamental = cfg.statusFundamental;
        }
      } else if (modo === "misto") {
        const isFund = idx < Math.floor(cfg.qtd * cfg.pctFundamental / 100);
        if (isFund) {
          if (anosF.length > 0) {
            const ano = cfg.anoFundamental === "random" ? pick(anosF) : cfg.anoFundamental;
            payload.ano_escolar = ano;
            payload.status_escolar_fundamental = cfg.statusFundamental;
          }
        } else {
          if (cursoMedioAlvo && anosMedio.length > 0) {
            const ano = cfg.anoMedio === "random" ? pick(anosMedio) : cfg.anoMedio;
            payload.ano_escolar_medio = ano;
            payload.status_escolar_medio = cfg.statusMedio;
            payload.curso_medio_id = cursoMedioAlvo.id;
          }
        }
      }

      return payload;
    });

    // ✅ Envia array puro — a API espera array de CriarEstudanteRequest
    const { ok, data } = await callApi("POST", "/academia/estudante/register/async", items, academia.token);
    if (!ok) {
      const errMsg = (data as any)?.message || (data as any)?.error || 'Erro ao submeter';
      addLog(`  ✗ Erro ao submeter: ${errMsg}`, "err");
      return;
    }
    const jobId = (data as any)?.job_id;
    if (!jobId) { addLog(`  ✗ Job ID não retornado`, "err"); return; }
    addLog(`  ⏳ Job ${jobId} criado (${(data as any)?.total_items} estudantes) — aguardando conclusão...`, "info");

    const result = await acompanharJob(jobId, "Estudantes");
    addLog(`Estudantes: ${result.ok} ✓  ${result.err} ✗`, result.ok > 0 ? "ok" : "err");
    await sleep(3000);
    await refreshData();
  };

  // ─── Vincular Estudantes a Turmas ─────────────────────────────────────────────
  const vincularEstudantesATurmas = async () => {
    if (!academia || turmas.length === 0 || estudantes.length === 0) {
      addLog("Sem turmas ou estudantes disponíveis", "warn");
      return;
    }

    const estudantesEmTurma = new Set(turmas.flatMap(t => t.estudantes));
    const semTurma = estudantes.filter(e => !estudantesEmTurma.has(e.codigo_estudante));

    if (semTurma.length === 0) {
      addLog("Todos os estudantes já estão vinculados a alguma turma", "info");
      return;
    }

    addLog(`Vinculando ${semTurma.length} estudante(s) sem turma via async...`, "step");

    const turmasAtivas = turmas.filter(t => t.status !== "inativo" && t.status !== "deletado");
    let turmasAlvo: Turma[];

    if (vincularConfig.turmaCodigo !== "random") {
      const turmaEspecifica = turmasAtivas.find(t => t.codigo_turma === vincularConfig.turmaCodigo);
      if (!turmaEspecifica) {
        addLog(`Turma "${vincularConfig.turmaCodigo}" não encontrada ou inativa`, "err");
        return;
      }
      turmasAlvo = [turmaEspecifica];
      addLog(`  Usando turma específica: ${turmaEspecifica.codigo_turma} (${turmaEspecifica.nivel})`, "dim");
    } else {
      turmasAlvo = turmasAtivas;
      addLog(`  Distribuindo entre ${turmasAlvo.length} turma(s) — cada estudante vai para APENAS UMA turma`, "dim");
    }

    if (turmasAlvo.length === 0) {
      addLog("Nenhuma turma ativa disponível", "err");
      return;
    }

    // Cada estudante é atribuído a exactamente UMA turma (round-robin)
    // A API /academia/turma/estudante/async espera array de {codigo_turma, codigo_estudante}
    const items: { codigo_turma: string; codigo_estudante: string }[] = semTurma.map((e, idx) => ({
      codigo_turma: turmasAlvo[idx % turmasAlvo.length].codigo_turma,
      codigo_estudante: e.codigo_estudante,
    }));

    // Log da distribuição
    const distribuicao = items.reduce<Record<string, number>>((acc, v) => {
      acc[v.codigo_turma] = (acc[v.codigo_turma] || 0) + 1;
      return acc;
    }, {});
    Object.entries(distribuicao).forEach(([turma, qtd]) => {
      addLog(`    • ${turma}: ${qtd} estudante(s)`, "dim");
    });

    // ✅ Envia array puro — a API espera array de {codigo_turma, codigo_estudante}
    const { ok, data } = await callApi("POST", "/academia/turma/estudante/async", items, academia.token);
    if (!ok) {
      const errMsg = (data as any)?.message || (data as any)?.error || 'Erro ao submeter';
      addLog(`  ✗ Erro ao submeter: ${errMsg}`, "err");
      return;
    }
    const jobId = (data as any)?.job_id;
    if (!jobId) { addLog(`  ✗ Job ID não retornado`, "err"); return; }
    addLog(`  ⏳ Job ${jobId} criado (${items.length} vínculos) — aguardando...`, "info");

    const result = await acompanharJob(jobId, "Vínculos");
    addLog(`Vínculos: ${result.ok} ✓  ${result.err} ✗`, result.ok > 0 ? "ok" : "err");
    await sleep(2000);
    await refreshData();
  };

  // ─── Gerar Notas ─────────────────────────────────────────────────────────────
  const gerarNotas = async () => {
    if (!academia || materias.length === 0 || estudantes.length === 0) {
      addLog("Sem matérias ou estudantes — crie-os primeiro", "warn");
      return;
    }
    if (!academia.ano_letivo) { addLog("Academia sem ano letivo configurado", "err"); return; }

    const { qtdEstudantes, periodo: periodoConfig } = notaConfig;
    const total = qtdEstudantes > 0 ? Math.min(qtdEstudantes, estudantes.length) : estudantes.length;
    const sample = estudantes.slice(0, total);
    addLog(`Gerando notas para ${sample.length} estudante(s) via async...`, "step");

    const tipoNota = academia.tipo === "superior" ? "superior" : "escolar";
    const periodosEscolares = ["1_trimestre", "2_trimestre", "3_trimestre"];
    const batch: any[] = [];

    for (const est of sample) {
      if (cancelRef.current) break;

      const { ok: rOk, data: notasData } = await callApi("GET", `/notas-estudante/${est.codigo_estudante}`, undefined, academia.token);
      const notasExistentes = new Set<string>();
      if (rOk) {
        const notas: any[] = (notasData as any)?.notas || [];
        const anoLetivo = academia.ano_letivo;
        for (const n of notas) {
          if (n.ano_lectivo === anoLetivo) {
            notasExistentes.add(`${n.materia_disciplinar_id}|${n.periodo}|${n.tipo}|${n.categoria}`);
          }
        }
      }

      const materiasTipo = materias.filter(m => m.type === tipoNota);
      if (materiasTipo.length === 0) continue;
      const materiasSample = pickN(materiasTipo, Math.min(3, materiasTipo.length));

      for (const mat of materiasSample) {
        let periodos: string[];
        if (academia.tipo === "superior") {
          if (mat.periodo) {
            periodos = [mat.periodo];
          } else {
            const cursoMat = cursos.find(c => c.id === mat.curso_id);
            periodos = cursoMat?.periodos || ["1_semestre"];
          }
        } else {
          periodos = periodoConfig !== "random" ? [periodoConfig] : periodosEscolares;
        }

        const categoria = tipoNota === "escolar" ? "nota_escola" : "nota_exame";

        for (const p of periodos) {
          const chave = `${mat.id}|${p}|${tipoNota}|${categoria}`;
          if (notasExistentes.has(chave)) continue;
          batch.push({
            codigo_estudante: est.codigo_estudante,
            periodo: p,
            materia_disciplinar_id: mat.id,
            tipo: tipoNota,
            categoria,
            nota: parseFloat((rnd(8, 20) + Math.random()).toFixed(1)),
          });
        }
      }
      await sleep(30);
    }

    if (batch.length === 0) { addLog("Nenhuma nota nova para registrar", "info"); return; }
    addLog(`  Enviando ${batch.length} nota(s) via async...`, "dim");

    // ✅ Envia array puro — a API espera array de RegistrarNotasRequest
    const { ok, data } = await callApi("POST", "/academia/notas-aluno/async", batch, academia.token);
    if (!ok) {
      const errMsg = (data as any)?.message || (data as any)?.error || 'Erro ao submeter';
      addLog(`  ✗ Erro: ${errMsg}`, "err");
      return;
    }
    const jobId = (data as any)?.job_id;
    if (!jobId) { addLog(`  ✗ Job ID não retornado`, "err"); return; }
    addLog(`  ⏳ Job ${jobId} (${batch.length} notas) — aguardando...`, "info");

    const result = await acompanharJob(jobId, "Notas");
    addLog(`Notas: ${result.ok} ✓  ${result.err} ✗`, result.ok > 0 ? "ok" : "err");
  };

  // ─── Gerar Faltas ─────────────────────────────────────────────────────────────
  const gerarFaltas = async () => {
    if (!academia || materias.length === 0 || estudantes.length === 0) {
      addLog("Sem matérias ou estudantes", "warn");
      return;
    }
    if (!academia.ano_letivo) { addLog("Academia sem ano letivo configurado", "err"); return; }

    const { qtdEstudantes } = faltaConfig;
    const total = qtdEstudantes > 0 ? Math.min(qtdEstudantes, estudantes.length) : estudantes.length;
    const sample = estudantes.slice(0, total);
    addLog(`Gerando faltas para ${sample.length} estudante(s) via async...`, "step");

    const tipoMaterias = academia.tipo === "superior" ? "superior" : "escolar";
    const materiasTipo = materias.filter(m => m.type === tipoMaterias);

    if (materiasTipo.length === 0) {
      addLog(`  ✗ Nenhuma matéria do tipo "${tipoMaterias}" ativa`, "err");
      return;
    }

    const batch: any[] = [];

    for (const est of sample) {
      if (cancelRef.current) break;

      const { ok: rOk, data: faltasData } = await callApi("GET", `/faltas-estudante/${est.codigo_estudante}`, undefined, academia.token);
      const faltasExistentes = new Set<string>();
      if (rOk) {
        const faltas: any[] = (faltasData as any)?.faltas || [];
        const anoLetivo = academia.ano_letivo;
        for (const f of faltas) {
          if (f.ano_lectivo === anoLetivo) {
            faltasExistentes.add(`${f.materia_disciplinar_id}|${f.data}`);
          }
        }
      }

      const materiasSample = pickN(materiasTipo, Math.min(2, materiasTipo.length));
      for (const mat of materiasSample) {
        const dataFalta = pick(DATAS_FALTA);
        const chave = `${mat.id}|${dataFalta}`;
        if (faltasExistentes.has(chave)) continue;
        batch.push({
          codigo_estudante: est.codigo_estudante,
          data: dataFalta,
          materia_disciplinar_id: mat.id,
          quantidade: rnd(1, 3),
        });
      }
      await sleep(30);
    }

    if (batch.length === 0) { addLog("Nenhuma falta nova para registrar", "info"); return; }
    addLog(`  Enviando ${batch.length} falta(s) via async...`, "dim");

    // ✅ Envia array puro — a API espera array de RegistrarFaltasRequest
    const { ok, data } = await callApi("POST", "/academia/faltas-aluno/async", batch, academia.token);
    if (!ok) {
      const errMsg = (data as any)?.message || (data as any)?.error || 'Erro ao submeter';
      addLog(`  ✗ Erro: ${errMsg}`, "err");
      return;
    }
    const jobId = (data as any)?.job_id;
    if (!jobId) { addLog(`  ✗ Job ID não retornado`, "err"); return; }
    addLog(`  ⏳ Job ${jobId} (${batch.length} faltas) — aguardando...`, "info");

    const result = await acompanharJob(jobId, "Faltas");
    addLog(`Faltas: ${result.ok} ✓  ${result.err} ✗`, result.ok > 0 ? "ok" : "err");
  };

  // ─── Gerar Avaliações Finais ───────────────────────────────────────────────────
  const gerarAvaliacoes = async () => {
    if (!academia || estudantes.length === 0) { addLog("Sem estudantes disponíveis", "warn"); return; }
    if (!academia.ano_letivo) { addLog("Academia sem ano letivo configurado", "err"); return; }

    const { tipoEnsino, aprovPct } = avalConfig;
    const sample = [...estudantes];
    const nAprov = Math.floor(sample.length * aprovPct / 100);
    addLog(`Gerando avaliações finais (${tipoEnsino}) para ${sample.length} estudante(s) via async...`, "step");

    const batch: any[] = [];

    for (let idx = 0; idx < sample.length; idx++) {
      const est = sample[idx];
      const aprovado = idx < nAprov;
      let nivelAtual = "";
      let proximoNivel: string | undefined;

      if (tipoEnsino === "fundamental") {
        const anosF = (academia.anos_academicos || []).filter(a => a.includes("fundamental")).sort();
        const anoAtual = est.ano_escolar || (anosF.length > 0 ? pick(anosF) : "1_ano_fundamental");
        nivelAtual = anoAtual;
        const idx2 = anosF.indexOf(anoAtual);
        if (aprovado && idx2 >= 0 && idx2 < anosF.length - 1) {
          proximoNivel = anosF[idx2 + 1];
        }
      } else if (tipoEnsino === "medio") {
        const c = cursos.find(x => x.type === "medio" && x.status === "ativo");
        const anos = c?.anos_academicos?.sort() || [];
        if (anos.length === 0) {
          addLog(`  ! Nenhum curso médio ativo para avaliação — ignorando estudante ${est.codigo_estudante}`, "warn");
          continue;
        }
        const anoAtual = est.ano_escolar_medio || anos[0];
        nivelAtual = anoAtual;
        const idx2 = anos.indexOf(anoAtual);
        if (aprovado && idx2 >= 0 && idx2 < anos.length - 1) {
          proximoNivel = anos[idx2 + 1];
        }
      } else if (tipoEnsino === "superior") {
        const c = cursos.find(x => x.type === "superior" && x.status === "ativo");
        const anos = c?.anos_academicos?.sort() || [];
        if (anos.length === 0) {
          addLog(`  ! Nenhum curso superior ativo para avaliação — ignorando estudante ${est.codigo_estudante}`, "warn");
          continue;
        }
        const anoAtual = est.ano_superior || anos[0];
        nivelAtual = anoAtual;
        const idx2 = anos.indexOf(anoAtual);
        if (aprovado && idx2 >= 0 && idx2 < anos.length - 1) {
          proximoNivel = anos[idx2 + 1];
        }
      }

      if (!nivelAtual) continue;

      const item: any = {
        codigo_estudante: est.codigo_estudante,
        tipo_ensino: tipoEnsino,
        nivel_ano_academico_atual: nivelAtual,
        aprovado,
        observacao: "Avaliação gerada pelo painel de testes",
      };
      if (aprovado && proximoNivel) {
        item.proximo_ano_academico = proximoNivel;
      }

      batch.push(item);
    }

    if (batch.length === 0) { addLog("Nenhuma avaliação para enviar", "warn"); return; }

    // ✅ Envia array puro — a API espera array de RegistrarAvaliacaoFinalRequest
    const { ok, data } = await callApi("POST", "/academia/avaliacao-final/async", batch, academia.token);
    if (!ok) {
      const errMsg = (data as any)?.message || (data as any)?.error || 'Erro ao submeter';
      addLog(`  ✗ Erro: ${errMsg}`, "err");
      return;
    }
    const jobId = (data as any)?.job_id;
    if (!jobId) { addLog(`  ✗ Job ID não retornado`, "err"); return; }
    addLog(`  ⏳ Job ${jobId} (${batch.length} avaliações) — aguardando...`, "info");

    const result = await acompanharJob(jobId, "Avaliações");
    addLog(`Avaliações: ${result.ok} ✓  ${result.err} ✗  (${nAprov} aprovações estimadas de ${sample.length} total)`, result.ok > 0 ? "ok" : "err");
  };

  // ─── Configurar Ano Letivo ─────────────────────────────────────────────────────
  const configurarAnoLetivo = async () => {
    if (!academia) return;
    const ano = "2025_2026";
    const tipo = academia.tipo === "superior" ? "superior" : "escola";
    const { ok, data } = await callApi("POST", "/academia/ano-letivo", { ano_letivo: ano, tipo }, academia.token);
    if (ok) {
      addLog(`Ano letivo ${ano} (tipo: ${tipo}) configurado ✓`, "ok");
      setAcademia(prev => prev ? { ...prev, ano_letivo: ano } : prev);
    } else {
      const errMsg = (data as any)?.message || (data as any)?.error || 'Erro desconhecido';
      addLog(`Ano letivo: ${errMsg}`, "warn");
    }
  };

  // ─── Derived ──────────────────────────────────────────────────────────────────

  const estudantesEmTurmaSet = new Set(turmas.flatMap(t => t.estudantes));
  const estudantesSemTurma = estudantes.filter(e => !estudantesEmTurmaSet.has(e.codigo_estudante));
  const tiposCursoDisp = academia ? tiposCursoValidos(academia) : [];
  const tiposMateriaDisp = academia ? tiposMateriaValidos(academia) : [];
  const tiposEnsinoDisp = academia ? tiposEnsinoDisponiveis(academia, cursos) : [];
  const anosDispFundamental = academia?.anos_academicos?.filter(a => a.includes("fundamental")) || [];
  const modo = academia ? getEscolaMode(academia) : "fundamental";

  const tipoCursoTurma = academia?.tipo === "superior" ? "superior" : "medio";
  const cursosParaTurma = cursos.filter(c => c.type === tipoCursoTurma && c.status === "ativo");
  const cursosMedioAtivos = cursos.filter(c => c.type === "medio" && c.status === "ativo");
  const cursosSuperiorAtivos = cursos.filter(c => c.type === "superior" && c.status === "ativo");

  const niveisParaTurma: string[] = (() => {
    const cursoEscolhido = turmaConfig.cursoId !== "random"
      ? cursosParaTurma.find(c => c.id === turmaConfig.cursoId)
      : undefined;
    if (cursoEscolhido) return cursoEscolhido.anos_academicos;
    const pool: string[] = [];
    if (academia?.tipo === "escola") {
      if (academia.nivel === "fundamental" || academia.nivel === "misto") {
        pool.push(...(academia.anos_academicos?.filter(a => a.includes("fundamental")) || []));
      }
      if (academia.nivel === "medio" || academia.nivel === "misto") {
        for (const c of cursosParaTurma) pool.push(...c.anos_academicos);
      }
    } else if (academia?.tipo === "superior") {
      for (const c of cursosParaTurma) pool.push(...c.anos_academicos);
    }
    return [...new Set(pool)];
  })();

  // ─── Render ─────────────────────────────────────────────────────────────────

  const logColors: Record<LogLevel, string> = {
    ok: "#4ade80", err: "#f87171", warn: "#facc15", info: "#94a3b8", step: "#60a5fa", dim: "#475569"
  };
  const logIcons: Record<LogLevel, string> = { ok: "✓", err: "✗", warn: "!", info: "·", step: "▶", dim: "·" };

  if (authError) {
    return (
      <div style={{ minHeight: "100vh", background: "#020817", color: "#e2e8f0", padding: 24, fontFamily: "'JetBrains Mono', monospace", display: "flex", alignItems: "center", justifyContent: "center" }}>
        <div style={{ maxWidth: 480, textAlign: "center" }}>
          <div style={{ fontSize: 48, marginBottom: 16 }}>🔒</div>
          <h1 style={{ fontSize: 20, fontWeight: 700, color: "#f87171", marginBottom: 12 }}>Acesso Restrito</h1>
          <p style={{ fontSize: 14, color: "#94a3b8", lineHeight: 1.6 }}>{authError}</p>
        </div>
      </div>
    );
  }

  if (!academia) {
    return (
      <div style={{ minHeight: "100vh", background: "#020817", color: "#e2e8f0", padding: 24, fontFamily: "'JetBrains Mono', monospace", display: "flex", alignItems: "center", justifyContent: "center" }}>
        <div style={{ fontSize: 14, color: "#475569" }}>Carregando dados da academia...</div>
      </div>
    );
  }

  const Section = ({ title, children, badge }: { title: string; children: React.ReactNode; badge?: string }) => (
    <div style={{ border: "1px solid #1e293b", borderRadius: 12, padding: 20, background: "#0f172a", marginBottom: 16 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 16 }}>
        <h3 style={{ margin: 0, fontSize: 14, fontWeight: 700, color: "#e2e8f0", letterSpacing: "0.05em", textTransform: "uppercase" }}>{title}</h3>
        {badge && <span style={{ fontSize: 11, padding: "2px 8px", background: "#1e3a5f", color: "#60a5fa", borderRadius: 20, fontWeight: 600 }}>{badge}</span>}
      </div>
      {children}
    </div>
  );

  const SubSection = ({ title, children }: { title: string; children: React.ReactNode }) => (
    <div style={{ border: "1px solid #1e3a5f", borderRadius: 8, padding: "12px 14px", background: "#0a1929", marginBottom: 12 }}>
      <p style={{ margin: "0 0 10px", fontSize: 11, fontWeight: 700, color: "#60a5fa", textTransform: "uppercase", letterSpacing: "0.06em" }}>{title}</p>
      {children}
    </div>
  );

  const Row = ({ children }: { children: React.ReactNode }) => (
    <div style={{ display: "flex", gap: 12, flexWrap: "wrap", alignItems: "flex-end", marginBottom: 12 }}>{children}</div>
  );

  const Field = ({ label, children }: { label: string; children: React.ReactNode }) => (
    <div style={{ display: "flex", flexDirection: "column", gap: 4, minWidth: 140 }}>
      <label style={{ fontSize: 11, color: "#64748b", fontWeight: 600, textTransform: "uppercase", letterSpacing: "0.05em" }}>{label}</label>
      {children}
    </div>
  );

  const Sel = (props: React.SelectHTMLAttributes<HTMLSelectElement> & { style?: React.CSSProperties }) => (
    <select {...props} style={{ background: "#1e293b", border: "1px solid #334155", color: "#e2e8f0", borderRadius: 8, padding: "6px 10px", fontSize: 13, cursor: "pointer", ...props.style }} />
  );

  const Inp = (props: React.InputHTMLAttributes<HTMLInputElement>) => (
    <input {...props} style={{ background: "#1e293b", border: "1px solid #334155", color: "#e2e8f0", borderRadius: 8, padding: "6px 10px", fontSize: 13, width: props.type === "number" ? 80 : undefined }} />
  );

  const Btn = ({ onClick, children, color = "#2563eb", disabled = false }: { onClick: () => void; children: React.ReactNode; color?: string; disabled?: boolean }) => (
    <button onClick={onClick} disabled={disabled || running} style={{ background: disabled || running ? "#1e293b" : color, color: disabled || running ? "#475569" : "#fff", border: "none", borderRadius: 8, padding: "8px 18px", fontSize: 13, fontWeight: 600, cursor: disabled || running ? "not-allowed" : "pointer", transition: "all 0.15s" }}>
      {children}
    </button>
  );

  const periodosNotaDisponiveis = academia.tipo !== "superior" ? [
    { value: "random", label: "Todos os trimestres" },
    { value: "1_trimestre", label: "1º Trimestre" },
    { value: "2_trimestre", label: "2º Trimestre" },
    { value: "3_trimestre", label: "3º Trimestre" },
  ] : [];

  const anosMedioParaConfig = cursosMedioAtivos.length > 0
    ? (estudanteConfig.cursoMedioId === "random"
        ? cursosMedioAtivos[0]?.anos_academicos || []
        : cursos.find(c => c.id === estudanteConfig.cursoMedioId)?.anos_academicos || [])
    : [];

  const anosSuperiorParaConfig = cursosSuperiorAtivos.length > 0
    ? (estudanteConfig.cursoSuperiorId === "random"
        ? cursosSuperiorAtivos[0]?.anos_academicos || []
        : cursos.find(c => c.id === estudanteConfig.cursoSuperiorId)?.anos_academicos || [])
    : [];

  return (
    <div style={{ minHeight: "100vh", background: "#020817", color: "#e2e8f0", padding: 24, fontFamily: "'JetBrains Mono', 'Fira Code', 'Courier New', monospace" }}>
      <div style={{ maxWidth: 1200, margin: "0 auto" }}>
        <div style={{ marginBottom: 28 }}>
          <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: "#f8fafc", letterSpacing: "-0.02em" }}>
            <span style={{ color: "#3b82f6" }}>⬡</span> Painel de Testes
          </h1>
          <p style={{ margin: "6px 0 0", fontSize: 13, color: "#64748b" }}>
            Academia: <span style={{ color: "#60a5fa", fontWeight: 700 }}>{academia.codigo}</span>
            {" · "}{academia.tipo}{academia.nivel ? ` · ${academia.nivel}` : ""}
            {academia.ano_letivo ? ` · Ano letivo: ${academia.ano_letivo.replace("_", "/")}` : " · ⚠ sem ano letivo"}
            {" · "}<span style={{ color: "#facc15" }}>⚡ Modo Assíncrono</span>
          </p>
        </div>

        <div style={{ display: "grid", gridTemplateColumns: "280px 1fr", gap: 20, alignItems: "start" }}>
          {/* ─── Estado ──────────────────────────────────────────────────── */}
          <div>
            <Section title="Estado atual">
              {[
                { label: "Cursos", val: cursos.length, icon: "📚" },
                { label: "Matérias ativas", val: materias.length, icon: "📖" },
                { label: "Turmas", val: turmas.length, icon: "🏫" },
                { label: "Estudantes", val: estudantes.length, icon: "👥" },
                { label: "Sem turma", val: estudantesSemTurma.length, icon: "⚠️", warn: estudantesSemTurma.length > 0 },
              ].map(s => (
                <div key={s.label} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "8px 0", borderBottom: "1px solid #1e293b" }}>
                  <span style={{ fontSize: 13, color: "#94a3b8" }}>{s.icon} {s.label}</span>
                  <span style={{ fontSize: 16, fontWeight: 800, color: (s as any).warn ? "#facc15" : s.val > 0 ? "#4ade80" : "#ef4444" }}>{s.val}</span>
                </div>
              ))}
              <div style={{ marginTop: 12 }}>
                <Row>
                  <Btn onClick={() => withLoading(async () => { await configurarAnoLetivo(); })} color="#0f4c75">
                    {academia.ano_letivo ? "✓ Ano letivo ok" : "Configurar ano letivo"}
                  </Btn>
                  <Btn onClick={() => refreshData()} color="#334155">↻ Atualizar</Btn>
                </Row>
              </div>
            </Section>
          </div>

          {/* ─── Operações ───────────────────────────────────────────────── */}
          <div>
            {/* Cursos */}
            {tiposCursoDisp.length > 0 && (
              <Section title="Cursos" badge={`${cursos.length} criados`}>
                <Row>
                  <Field label="Tipo">
                    <Sel value={cursoConfig.tipo} onChange={e => setCursoConfig(p => ({ ...p, tipo: e.target.value as any }))}>
                      {tiposCursoDisp.map(t => (<option key={t.value} value={t.value}>{t.label}</option>))}
                    </Sel>
                  </Field>
                  <Field label="Qtd">
                    <Inp type="number" min={1} max={5} value={cursoConfig.qtd}
                      onChange={e => setCursoConfig(p => ({ ...p, qtd: +e.target.value }))} />
                  </Field>
                  <Btn onClick={() => withLoading(gerarCursos)} color="#7c3aed">Gerar Cursos</Btn>
                </Row>
                {cursos.length > 0 && (
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginTop: 4 }}>
                    {cursos.map(c => (
                      <span key={c.id} style={{ fontSize: 11, padding: "3px 8px", background: "#1e293b", borderRadius: 20, color: "#94a3b8" }}>
                        {c.nome} <span style={{ color: "#475569" }}>({c.type})</span>
                      </span>
                    ))}
                  </div>
                )}
              </Section>
            )}

            {/* Matérias */}
            <Section title="Matérias Disciplinares" badge={`${materias.length} ativas`}>
              {tiposMateriaDisp.length === 0 ? (
                <p style={{ color: "#475569", fontSize: 12, margin: 0 }}>Tipo de academia não suporta matérias nesta configuração.</p>
              ) : (
                <>
                  <Row>
                    <Field label="Tipo">
                      <Sel value={materiaConfig.tipo} onChange={e => { const newTipo = e.target.value as any; setMateriaConfig(p => ({ ...p, tipo: newTipo, cursoId: "auto" })); }}>
                        {tiposMateriaDisp.map(t => (<option key={t.value} value={t.value}>{t.label}</option>))}
                      </Sel>
                    </Field>
                    {(materiaConfig.tipo === "medio" || materiaConfig.tipo === "superior") && (
                      <Field label="Curso">
                        <Sel value={materiaConfig.cursoId} onChange={e => setMateriaConfig(p => ({ ...p, cursoId: e.target.value }))}>
                          <option value="auto">Auto (primeiro ativo)</option>
                          {cursos.filter(c => c.type === materiaConfig.tipo && c.status === "ativo").map(c => <option key={c.id} value={c.id}>{c.nome}</option>)}
                        </Sel>
                      </Field>
                    )}
                    <Field label="Qtd">
                      <Inp type="number" min={1} max={10} value={materiaConfig.qtd}
                        onChange={e => setMateriaConfig(p => ({ ...p, qtd: +e.target.value }))} />
                    </Field>
                    <Btn
                      onClick={() => withLoading(gerarMaterias)}
                      color="#7c3aed"
                      disabled={(materiaConfig.tipo === "medio" || materiaConfig.tipo === "superior") && cursos.filter(c => c.type === materiaConfig.tipo && c.status === "ativo").length === 0}
                    >
                      Gerar Matérias
                    </Btn>
                  </Row>
                  {materiaConfig.tipo === "superior" && (
                    <p style={{ margin: "4px 0 0", fontSize: 11, color: "#facc15" }}>
                      ⚠ Matérias superiores nascem inativas — período definido automaticamente antes de ativar
                    </p>
                  )}
                </>
              )}
            </Section>

            {/* Turmas */}
            <Section title="Turmas" badge={`${turmas.length} criadas`}>
              {academia.tipo === "superior" && cursosParaTurma.length === 0 ? (
                <p style={{ color: "#f87171", fontSize: 12, margin: 0 }}>✗ Academia superior sem cursos ativos — crie cursos primeiro</p>
              ) : academia.nivel === "medio" && cursosParaTurma.length === 0 ? (
                <p style={{ color: "#f87171", fontSize: 12, margin: 0 }}>✗ Academia de nível médio sem cursos ativos — crie cursos primeiro</p>
              ) : (
                <>
                  <Row>
                    <Field label="Quantidade">
                      <Inp type="number" min={1} max={20} value={turmaConfig.qtd} onChange={e => setTurmaConfig(p => ({ ...p, qtd: +e.target.value }))} />
                    </Field>
                    <Field label="Turno">
                      <Sel value={turmaConfig.turno} onChange={e => setTurmaConfig(p => ({ ...p, turno: e.target.value }))}>
                        <option value="random">Aleatório</option>
                        <option value="manha">Manhã</option>
                        <option value="tarde">Tarde</option>
                        <option value="noite">Noite</option>
                      </Sel>
                    </Field>
                    {cursosParaTurma.length > 0 && (
                      <Field label="Curso">
                        <Sel
                          value={turmaConfig.cursoId}
                          onChange={e => setTurmaConfig(p => ({ ...p, cursoId: e.target.value, nivel: "random" }))}
                          style={{ minWidth: 200 }}
                        >
                          <option value="random">Aleatório (todos os cursos)</option>
                          {cursosParaTurma.map(c => (
                            <option key={c.id} value={c.id}>{c.nome}</option>
                          ))}
                        </Sel>
                      </Field>
                    )}
                    <Field label="Nível">
                      <Sel value={turmaConfig.nivel} onChange={e => setTurmaConfig(p => ({ ...p, nivel: e.target.value }))}>
                        <option value="random">Aleatório</option>
                        {(academia.nivel === "fundamental" || academia.nivel === "misto") &&
                          (academia.anos_academicos || []).filter(a => a.includes("fundamental")).map(a => (
                            <option key={a} value={a}>{a.replace(/_ano_fundamental$/, "º Fundamental")}</option>
                          ))
                        }
                        {niveisParaTurma.filter(a => !a.includes("fundamental")).map(a => (
                          <option key={a} value={a}>
                            {a.includes("medio") ? a.replace(/_ano_medio$/, "º Médio") : a.replace(/_ano_superior$/, "º Superior")}
                          </option>
                        ))}
                      </Sel>
                    </Field>
                    <Btn onClick={() => withLoading(gerarTurmas)} color="#0891b2">Gerar Turmas</Btn>
                  </Row>
                </>
              )}
            </Section>

            {/* Estudantes */}
            <Section title="Estudantes" badge={`${estudantes.length} cadastrados`}>
              <Row>
                <Field label="Quantidade">
                  <Inp type="number" min={1} max={1000} value={estudanteConfig.qtd}
                    onChange={e => setEstudanteConfig(p => ({ ...p, qtd: +e.target.value }))} />
                </Field>
              </Row>

              {(modo === "fundamental" || modo === "misto") && (
                <SubSection title="Ensino Fundamental">
                  <Row>
                    <Field label="Ano escolar">
                      <Sel value={estudanteConfig.anoFundamental}
                        onChange={e => setEstudanteConfig(p => ({ ...p, anoFundamental: e.target.value }))}>
                        <option value="random">Aleatório</option>
                        {anosDispFundamental.map(a => (
                          <option key={a} value={a}>{a.replace(/_ano_fundamental$/, "º Fundamental")}</option>
                        ))}
                      </Sel>
                    </Field>
                    <Field label="Status fundamental">
                      <Sel value={estudanteConfig.statusFundamental}
                        onChange={e => setEstudanteConfig(p => ({ ...p, statusFundamental: e.target.value }))}>
                        <option value="em_andamento">Em andamento</option>
                        <option value="inativo">Inativo</option>
                        <option value="finalizado">Finalizado</option>
                      </Sel>
                    </Field>
                  </Row>
                </SubSection>
              )}

              {(modo === "medio" || modo === "misto") && (
                <SubSection title="Ensino Médio">
                  {cursosMedioAtivos.length === 0 ? (
                    <p style={{ margin: 0, fontSize: 11, color: "#f87171" }}>✗ Nenhum curso médio ativo — crie e ative um curso médio primeiro</p>
                  ) : (
                    <Row>
                      <Field label="Curso médio">
                        <Sel value={estudanteConfig.cursoMedioId}
                          onChange={e => setEstudanteConfig(p => ({ ...p, cursoMedioId: e.target.value, anoMedio: "random" }))}>
                          <option value="random">Primeiro ativo</option>
                          {cursosMedioAtivos.map(c => (
                            <option key={c.id} value={c.id}>{c.nome}</option>
                          ))}
                        </Sel>
                      </Field>
                      <Field label="Ano médio">
                        <Sel value={estudanteConfig.anoMedio}
                          onChange={e => setEstudanteConfig(p => ({ ...p, anoMedio: e.target.value }))}>
                          <option value="random">Aleatório</option>
                          {anosMedioParaConfig.map(a => (
                            <option key={a} value={a}>{a.replace(/_ano_medio$/, "º Médio")}</option>
                          ))}
                        </Sel>
                      </Field>
                      <Field label="Status médio">
                        <Sel value={estudanteConfig.statusMedio}
                          onChange={e => setEstudanteConfig(p => ({ ...p, statusMedio: e.target.value }))}>
                          <option value="em_andamento">Em andamento</option>
                          <option value="inativo">Inativo</option>
                          <option value="finalizado">Finalizado</option>
                        </Sel>
                      </Field>
                    </Row>
                  )}
                </SubSection>
              )}

              {modo === "superior" && (
                <SubSection title="Ensino Superior">
                  {cursosSuperiorAtivos.length === 0 ? (
                    <p style={{ margin: 0, fontSize: 11, color: "#f87171" }}>✗ Nenhum curso superior ativo — crie e ative um curso superior primeiro</p>
                  ) : (
                    <Row>
                      <Field label="Curso superior">
                        <Sel value={estudanteConfig.cursoSuperiorId}
                          onChange={e => setEstudanteConfig(p => ({ ...p, cursoSuperiorId: e.target.value, anoSuperior: "random" }))}>
                          <option value="random">Primeiro ativo</option>
                          {cursosSuperiorAtivos.map(c => (
                            <option key={c.id} value={c.id}>{c.nome}</option>
                          ))}
                        </Sel>
                      </Field>
                      <Field label="Ano superior">
                        <Sel value={estudanteConfig.anoSuperior}
                          onChange={e => setEstudanteConfig(p => ({ ...p, anoSuperior: e.target.value }))}>
                          <option value="random">Aleatório</option>
                          {anosSuperiorParaConfig.map(a => (
                            <option key={a} value={a}>{a.replace(/_ano_superior$/, "º Superior")}</option>
                          ))}
                        </Sel>
                      </Field>
                      <Field label="Status superior">
                        <Sel value={estudanteConfig.statusSuperior}
                          onChange={e => setEstudanteConfig(p => ({ ...p, statusSuperior: e.target.value }))}>
                          <option value="em_andamento">Em andamento</option>
                          <option value="inativo">Inativo</option>
                          <option value="finalizado">Finalizado</option>
                        </Sel>
                      </Field>
                    </Row>
                  )}
                </SubSection>
              )}

              {modo === "misto" && (
                <SubSection title="Distribuição Misto">
                  <Row>
                    <Field label="% Fundamental">
                      <Inp type="number" min={0} max={100} value={estudanteConfig.pctFundamental}
                        onChange={e => setEstudanteConfig(p => ({ ...p, pctFundamental: +e.target.value }))} />
                    </Field>
                  </Row>
                  <p style={{ margin: "4px 0 0", fontSize: 11, color: "#64748b" }}>
                    ≈ {Math.floor(estudanteConfig.qtd * estudanteConfig.pctFundamental / 100)} fundamental
                    · {estudanteConfig.qtd - Math.floor(estudanteConfig.qtd * estudanteConfig.pctFundamental / 100)} médio
                  </p>
                </SubSection>
              )}

              <div style={{ marginTop: 8 }}>
                <Btn onClick={() => withLoading(gerarEstudantes)} color="#059669">
                  Gerar {estudanteConfig.qtd} Estudante(s) (async)
                </Btn>
              </div>
              <p style={{ margin: "8px 0 0", fontSize: 11, color: "#475569" }}>
                Usa endpoint assíncrono — limite: 1000 por job
              </p>
            </Section>

            {/* Vincular a Turmas */}
            {turmas.length > 0 && estudantes.length > 0 && (
              <Section title="Vincular Estudantes a Turmas" badge={estudantesSemTurma.length > 0 ? `${estudantesSemTurma.length} sem turma` : "todos vinculados"}>
                {estudantesSemTurma.length === 0 ? (
                  <p style={{ color: "#4ade80", fontSize: 12, margin: 0 }}>✓ Todos os estudantes já estão vinculados a turmas.</p>
                ) : (
                  <>
                    <Row>
                      <Field label="Turma alvo">
                        <Sel value={vincularConfig.turmaCodigo} onChange={e => setVincularConfig({ turmaCodigo: e.target.value })} style={{ minWidth: 220 }}>
                          <option value="random">Distribuir entre todas (round-robin)</option>
                          {turmas.filter(t => t.status !== "inativo" && t.status !== "deletado").map(t => (
                            <option key={t.codigo_turma} value={t.codigo_turma}>
                              {t.codigo_turma} — {t.nivel.replace(/_ano_(fundamental|medio|superior)$/, "º $1")} ({t.estudantes.length} alunos)
                            </option>
                          ))}
                        </Sel>
                      </Field>
                      <Btn onClick={() => withLoading(vincularEstudantesATurmas)} color="#0f4c75">
                        Vincular {estudantesSemTurma.length} → 1 turma cada (async)
                      </Btn>
                    </Row>
                    <p style={{ margin: "4px 0 0", fontSize: 11, color: "#475569" }}>
                      ✦ Cada estudante é vinculado a <strong style={{ color: "#60a5fa" }}>exactamente uma</strong> turma, independentemente do nível de ensino
                    </p>
                  </>
                )}
              </Section>
            )}

            {/* Notas */}
            <Section title="Notas" badge={materias.length === 0 ? "crie matérias primeiro" : undefined}>
              {academia.tipo === "superior" ? (
                <Row>
                  <Field label="Nº estudantes (0 = todos)">
                    <Inp type="number" min={0} max={estudantes.length || 100} value={notaConfig.qtdEstudantes}
                      onChange={e => setNotaConfig(p => ({ ...p, qtdEstudantes: +e.target.value }))} />
                  </Field>
                  <Btn onClick={() => withLoading(gerarNotas)} color="#b45309" disabled={materias.length === 0 || estudantes.length === 0}>Gerar Notas (async)</Btn>
                </Row>
              ) : (
                <Row>
                  <Field label="Nº estudantes (0 = todos)">
                    <Inp type="number" min={0} max={estudantes.length || 100} value={notaConfig.qtdEstudantes}
                      onChange={e => setNotaConfig(p => ({ ...p, qtdEstudantes: +e.target.value }))} />
                  </Field>
                  <Field label="Período">
                    <Sel value={notaConfig.periodo} onChange={e => setNotaConfig(p => ({ ...p, periodo: e.target.value }))}>
                      {periodosNotaDisponiveis.map(p => (<option key={p.value} value={p.value}>{p.label}</option>))}
                    </Sel>
                  </Field>
                  <Btn onClick={() => withLoading(gerarNotas)} color="#b45309" disabled={materias.length === 0 || estudantes.length === 0}>Gerar Notas (async)</Btn>
                </Row>
              )}
              <p style={{ margin: "4px 0 0", fontSize: 11, color: "#475569" }}>
                {academia.tipo === "superior" ? "Tipo: superior · Categoria: nota_exame · Período: definido por matéria" : "Tipo: escolar · Categoria: nota_escola · Períodos: trimestres do sistema"}
              </p>
            </Section>

            {/* Faltas */}
            <Section title="Faltas" badge={materias.length === 0 ? "crie matérias primeiro" : undefined}>
              <Row>
                <Field label="Nº estudantes (0 = todos)">
                  <Inp type="number" min={0} max={estudantes.length || 100} value={faltaConfig.qtdEstudantes}
                    onChange={e => setFaltaConfig(p => ({ ...p, qtdEstudantes: +e.target.value }))} />
                </Field>
                <Btn onClick={() => withLoading(gerarFaltas)} color="#b45309" disabled={materias.length === 0 || estudantes.length === 0}>Gerar Faltas (async)</Btn>
              </Row>
            </Section>

            {/* Avaliações Finais */}
            <Section title="Avaliações Finais" badge={estudantes.length === 0 ? "crie estudantes primeiro" : `${estudantes.length} estudantes`}>
              {tiposEnsinoDisp.length === 0 ? (
                <p style={{ color: "#475569", fontSize: 12, margin: 0 }}>
                  {academia.tipo === "escola" && (academia.nivel === "medio" || academia.nivel === "misto")
                    ? "Crie e ative cursos médios para habilitar avaliações de nível médio"
                    : "Crie e ative cursos para habilitar avaliações"}
                </p>
              ) : (
                <>
                  <Row>
                    <Field label="Tipo ensino">
                      <Sel value={avalConfig.tipoEnsino} onChange={e => setAvalConfig(p => ({ ...p, tipoEnsino: e.target.value }))}>
                        {tiposEnsinoDisp.map(t => (<option key={t.value} value={t.value}>{t.label}</option>))}
                      </Sel>
                    </Field>
                    <Field label="% Aprovação">
                      <Inp type="number" min={0} max={100} value={avalConfig.aprovPct}
                        onChange={e => setAvalConfig(p => ({ ...p, aprovPct: +e.target.value }))} />
                    </Field>
                    <Btn onClick={() => withLoading(gerarAvaliacoes)} color="#7c3aed" disabled={estudantes.length === 0}>
                      Avaliar TODOS ({estudantes.length}) async
                    </Btn>
                  </Row>
                  <p style={{ margin: 0, fontSize: 11, color: "#475569" }}>
                    {Math.floor(estudantes.length * avalConfig.aprovPct / 100)} aprovações estimadas de {estudantes.length}
                  </p>
                </>
              )}
            </Section>
          </div>
        </div>

        {/* ─── Log ───────────────────────────────────────────────────────── */}
        {logs.length > 0 && (
          <div style={{ marginTop: 20, border: "1px solid #1e293b", borderRadius: 12, overflow: "hidden" }}>
            <div style={{ background: "#0f172a", padding: "10px 16px", display: "flex", justifyContent: "space-between", alignItems: "center", borderBottom: "1px solid #1e293b" }}>
              <span style={{ fontSize: 12, color: "#475569", fontWeight: 600 }}>LOG · {logs.length} entradas</span>
              <div style={{ display: "flex", gap: 10 }}>
                {running && (
                  <button onClick={() => { cancelRef.current = true; addLog("Cancelando...", "warn"); }} style={{ background: "#7f1d1d", color: "#fca5a5", border: "none", borderRadius: 6, padding: "4px 12px", fontSize: 12, cursor: "pointer" }}>
                    ✕ Cancelar
                  </button>
                )}
                <button onClick={() => setLogs([])} style={{ background: "transparent", color: "#475569", border: "1px solid #1e293b", borderRadius: 6, padding: "4px 12px", fontSize: 12, cursor: "pointer" }}>
                  Limpar
                </button>
              </div>
            </div>
            <div style={{ background: "#020817", height: 320, overflowY: "auto", padding: 16, fontSize: 12, lineHeight: "1.8" }}>
              {logs.map((l, i) => (
                <div key={i} style={{ display: "flex", gap: 12, color: logColors[l.level] }}>
                  <span style={{ color: "#334155", minWidth: 64 }}>{l.ts}</span>
                  <span style={{ minWidth: 12 }}>{logIcons[l.level]}</span>
                  <span style={{ whiteSpace: "pre-wrap", wordBreak: "break-all" }}>{l.msg}</span>
                </div>
              ))}
              <div ref={logsEndRef} />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}