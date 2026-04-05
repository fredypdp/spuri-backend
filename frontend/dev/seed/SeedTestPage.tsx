"use client";
import { useState, useCallback, useRef, useEffect } from "react";
import { tokenStorage } from "@/lib/api";

// ─── Types ────────────────────────────────────────────────────────────────────

type LogLevel = "ok" | "err" | "warn" | "info" | "step" | "dim";
interface LogEntry { ts: string; level: LogLevel; msg: string; }

interface AcademiaInfo {
  codigo: string;
  nome: string;
  tipo: "escola" | "superior";
  nivel?: "fundamental" | "medio" | "misto";
  anos_academicos?: string[];
  ano_letivo?: string;
}

interface Materia { id: string; nome: string; type: string; anos_academicos: string[]; periodo?: string; }
interface Estudante { codigo_estudante: string; nome: string; }
interface Turma { codigo_turma: string; nivel: string; estudantes: string[]; }
interface Curso { id: string; nome: string; type: string; anos_academicos: string[]; periodos?: string[]; }

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

// ─── Main Component ───────────────────────────────────────────────────────────

export default function SeedTestPage() {
  const [academia, setAcademia] = useState<AcademiaInfo | null>(null);
  const [loadingPerfil, setLoadingPerfil] = useState(true);

  const [cursos, setCursos] = useState<Curso[]>([]);
  const [materias, setMaterias] = useState<Materia[]>([]);
  const [turmas, setTurmas] = useState<Turma[]>([]);
  const [estudantes, setEstudantes] = useState<Estudante[]>([]);

  const [cursoConfig, setCursoConfig] = useState({ tipo: "medio" as "medio"|"superior", qtd: 2 });
  const [materiaConfig, setMateriaConfig] = useState({ tipo: "fundamental" as "fundamental"|"medio"|"superior", qtd: 5, cursoId: "auto" });
  const [turmaConfig, setTurmaConfig] = useState({ qtd: 3, turno: "random", nivel: "random" });
  const [estudanteConfig, setEstudanteConfig] = useState({ qtd: 20, anoEscolar: "random", statusFund: "em_andamento" });
  const [notaConfig, setNotaConfig] = useState({ qtdEstudantes: 10 });
  const [faltaConfig, setFaltaConfig] = useState({ qtdEstudantes: 10 });
  const [avalConfig, setAvalConfig] = useState({ tipoEnsino: "fundamental", aprovPct: 70, qtdEstudantes: 10 });

  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [running, setRunning] = useState(false);
  const logsEndRef = useRef<HTMLDivElement>(null);
  const cancelRef = useRef(false);

  const addLog = useCallback((msg: string, level: LogLevel = "info") => {
    const ts = new Date().toLocaleTimeString("pt-PT", { hour: "2-digit", minute: "2-digit", second: "2-digit" });
    setLogs(prev => [...prev, { ts, level, msg }].slice(-800));
    setTimeout(() => logsEndRef.current?.scrollIntoView({ behavior: "smooth" }), 30);
  }, []);

  // Usa sempre o token da academia logada — nunca faz login próprio
  const callApi = useCallback(async (method: string, path: string, body?: unknown) => {
    const url = (process.env.NEXT_PUBLIC_API_URL || "") + path;
    const token = tokenStorage.get() || "";
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json", "Authorization": `Bearer ${token}` };
      const r = await fetch(url, { method, headers, body: body != null ? JSON.stringify(body) : undefined });
      const data = await r.json().catch(() => ({}));
      return { ok: r.status >= 200 && r.status < 300, status: r.status, data };
    } catch (e) {
      return { ok: false, status: 0, data: { error: String(e) } };
    }
  }, []);

  const loadPerfil = useCallback(async () => {
    setLoadingPerfil(true);
    const { ok, data } = await callApi("GET", "/meu-perfil");
    if (ok && (data as any).academia) {
      const a = (data as any).academia;
      setAcademia({
        codigo: a.codigo_academia,
        nome: a.nome,
        tipo: a.type,
        nivel: a.nivel_escolar,
        anos_academicos: a.anos_academicos || [],
        ano_letivo: a.ano_letivo,
      });
    } else {
      addLog("Não foi possível carregar o perfil da academia.", "err");
    }
    setLoadingPerfil(false);
  }, [callApi, addLog]);

  const refreshData = useCallback(async () => {
    const [rCursos, rMaterias, rTurmas, rEstudantes] = await Promise.all([
      callApi("GET", "/academia/cursos"),
      callApi("GET", "/academia/materias"),
      callApi("GET", "/academia/turmas"),
      callApi("GET", "/estudantes"),
    ]);
    setCursos((rCursos.data as any)?.cursos || []);
    setMaterias((rMaterias.data as any)?.materias?.filter((m: any) => m.status === "ativo") || []);
    setTurmas((rTurmas.data as any)?.turmas || []);
    setEstudantes((rEstudantes.data as any)?.estudantes || []);
    addLog(
      `Dados: ${(rCursos.data as any)?.total || 0} cursos · ${(rMaterias.data as any)?.total || 0} matérias · ${(rTurmas.data as any)?.turmas?.length || 0} turmas · ${(rEstudantes.data as any)?.total || 0} estudantes`,
      "dim"
    );
  }, [callApi, addLog]);

  useEffect(() => { loadPerfil(); }, [loadPerfil]);
  useEffect(() => { if (academia) refreshData(); }, [academia, refreshData]);

  const withLoading = async (fn: () => Promise<void>) => {
    setRunning(true);
    cancelRef.current = false;
    try { await fn(); } finally { setRunning(false); }
  };

  // ─── Operations ──────────────────────────────────────────────────────────────

  const gerarCursos = async () => {
    if (!academia) return;
    const { tipo, qtd } = cursoConfig;
    addLog(`Gerando ${qtd} curso(s) do tipo ${tipo}...`, "step");
    const templates = tipo === "medio" ? CURSOS_MEDIO : CURSOS_SUPERIOR;
    for (const t of pickN(templates, Math.min(qtd, templates.length))) {
      if (cancelRef.current) break;
      const payload: any = { nome: t.nome, type: tipo, anos_academicos: t.anos };
      if (tipo === "superior") payload.periodos = (t as any).periodos;
      const { ok, data } = await callApi("POST", "/academia/curso", payload);
      if (ok) {
        const id = (data as any).data?.id;
        addLog(`  ✓ Curso "${t.nome}" criado`, "ok");
        if (id) { await sleep(500); await callApi("PUT", `/academia/curso/${id}/ativar`); addLog(`    ✓ Ativado`, "dim"); }
      } else {
        addLog(`  ✗ "${t.nome}": ${(data as any)?.message || (data as any)?.error}`, "warn");
      }
      await sleep(300);
    }
    await sleep(3000);
    await refreshData();
    addLog("Cursos gerados ✓", "ok");
  };

  const gerarMaterias = async () => {
    if (!academia) return;
    const { tipo, qtd, cursoId } = materiaConfig;
    addLog(`Gerando ${qtd} matéria(s) do tipo ${tipo}...`, "step");
    const pool = tipo === "fundamental" ? MATERIAS_FUND : tipo === "medio" ? MATERIAS_MEDIO : MATERIAS_SUPERIOR;
    let anosDisponiveis: string[] = [];
    if (tipo === "fundamental") {
      anosDisponiveis = academia.anos_academicos?.filter(a => a.includes("fundamental")) || ["1_ano_fundamental"];
    } else {
      const curso = cursoId !== "auto" ? cursos.find(c => c.id === cursoId) : cursos.find(c => c.type === tipo);
      anosDisponiveis = curso?.anos_academicos || [`1_ano_${tipo}`];
    }
    let criadas = 0;
    for (const nome of pickN(pool, Math.min(qtd, pool.length))) {
      if (cancelRef.current) break;
      const payload: any = { nome, type: tipo, anos_academicos: [pick(anosDisponiveis)] };
      if (tipo !== "fundamental") {
        const curso = cursoId !== "auto" ? cursos.find(c => c.id === cursoId) : cursos.find(c => c.type === tipo);
        if (!curso) { addLog(`  ✗ Nenhum curso ${tipo} — crie cursos primeiro`, "warn"); continue; }
        payload.curso_id = curso.id;
      }
      const { ok, data } = await callApi("POST", "/academia/materia", payload);
      if (ok) {
        const id = (data as any).data?.id;
        criadas++;
        addLog(`  ✓ "${nome}" criada`, "ok");
        if (id && tipo === "superior") {
          const curso = cursoId !== "auto" ? cursos.find(c => c.id === cursoId) : cursos.find(c => c.type === "superior");
          if (curso?.periodos?.length) { await sleep(300); await callApi("PUT", `/academia/materia/${id}/periodo`, { periodo: pick(curso.periodos) }); }
        }
        if (id) { await sleep(500); await callApi("PUT", `/academia/materia/${id}/ativar`); }
      } else {
        addLog(`  ✗ "${nome}": ${(data as any)?.message || (data as any)?.error}`, "warn");
      }
      await sleep(300);
    }
    await sleep(3000);
    await refreshData();
    addLog(`${criadas} matéria(s) gerada(s) ✓`, "ok");
  };

  const gerarTurmas = async () => {
    if (!academia) return;
    addLog(`Gerando ${turmaConfig.qtd} turma(s)...`, "step");
    const niveisDisponiveis: string[] = [];
    if (academia.nivel === "fundamental" || academia.nivel === "misto")
      niveisDisponiveis.push(...(academia.anos_academicos?.filter(a => a.includes("fundamental")) || ["1_ano_fundamental"]));
    if (academia.nivel === "medio" || academia.nivel === "misto") {
      const c = cursos.find(c => c.type === "medio"); if (c) niveisDisponiveis.push(...c.anos_academicos);
    }
    if (academia.tipo === "superior") {
      const c = cursos.find(c => c.type === "superior"); if (c) niveisDisponiveis.push(...c.anos_academicos);
    }
    if (niveisDisponiveis.length === 0) niveisDisponiveis.push("1_ano_fundamental");
    let criadas = 0;
    for (let i = 0; i < turmaConfig.qtd; i++) {
      if (cancelRef.current) break;
      const nivel = turmaConfig.nivel === "random" ? pick(niveisDisponiveis) : turmaConfig.nivel;
      const turno = turmaConfig.turno === "random" ? pick([...TURNOS]) : turmaConfig.turno as typeof TURNOS[number];
      const payload: any = { codigo_turma: `T${rnd(1,9)}${String.fromCharCode(65+i)}${rnd(10,99)}`, nivel, turno };
      if (nivel.includes("medio")) { const c = cursos.find(x => x.type === "medio"); if (c) payload.curso_id = c.id; }
      else if (nivel.includes("superior")) { const c = cursos.find(x => x.type === "superior"); if (c) payload.curso_id = c.id; }
      const { ok, data } = await callApi("POST", "/academia/turma", payload);
      if (ok) { criadas++; addLog(`  ✓ Turma ${payload.codigo_turma} (${nivel}, ${turno})`, "ok"); }
      else addLog(`  ✗ ${payload.codigo_turma}: ${(data as any)?.message || (data as any)?.error}`, "warn");
      await sleep(200);
    }
    await sleep(2000);
    await refreshData();
    addLog(`${criadas} turma(s) gerada(s) ✓`, "ok");
  };

  const vincularEstudantesATurmas = async () => {
    if (turmas.length === 0 || estudantes.length === 0) { addLog("Sem turmas ou estudantes", "warn"); return; }
    addLog("Vinculando estudantes às turmas...", "step");
    const turmaSample = turmas.filter(t => t.estudantes.length < 40);
    if (turmaSample.length === 0) { addLog("Todas as turmas estão cheias", "warn"); return; }
    const vinculos = estudantes.slice(0, 50).map(e => ({ codigo_turma: pick(turmaSample).codigo_turma, codigo_estudante: e.codigo_estudante }));
    const { ok, data } = await callApi("POST", "/academia/turma/estudante/batch", vinculos);
    if (ok) { addLog(`${((data as any)?.items || []).filter((x: any) => x.sucesso).length} vínculos criados ✓`, "ok"); }
    else addLog(`Erro: ${(data as any)?.message}`, "err");
    await sleep(2000);
    await refreshData();
  };

  const gerarEstudantes = async () => {
    if (!academia) return;
    const { qtd, anoEscolar, statusFund } = estudanteConfig;
    addLog(`Gerando ${qtd} estudante(s)...`, "step");
    const anosDisp = academia.anos_academicos?.filter(a => a.includes("fundamental")) || ["1_ano_fundamental"];
    const batch: any[] = Array.from({ length: qtd }, () => {
      const { nome, genero } = gerarNome();
      const payload: any = { nome, genero, data_nascimento: gerarDataNasc() };
      if (academia.nivel === "fundamental" || academia.nivel === "misto") {
        payload.ano_escolar = anoEscolar === "random" ? pick(anosDisp) : anoEscolar;
        payload.status_escolar_fundamental = statusFund;
      } else if (academia.nivel === "medio") {
        payload.ano_escolar_medio = "1_ano_medio"; payload.status_escolar_medio = "em_andamento";
        const c = cursos.find(x => x.type === "medio"); if (c) payload.curso_medio_id = c.id;
      } else if (academia.tipo === "superior") {
        payload.ano_superior = "1_ano_superior"; payload.status_superior = "em_andamento";
        const c = cursos.find(x => x.type === "superior"); if (c) payload.curso_superior_id = c.id;
      }
      return payload;
    });
    let ok = 0, err = 0;
    for (let i = 0; i < batch.length; i += 50) {
      if (cancelRef.current) break;
      const { ok: rOk, data } = await callApi("POST", "/academia/estudante/register/batch", batch.slice(i, i + 50));
      if (rOk) { const items = (data as any)?.items || []; ok += items.filter((x: any) => x.sucesso).length; err += items.filter((x: any) => !x.sucesso).length; }
      else { err += Math.min(50, batch.length - i); }
      await sleep(500);
    }
    addLog(`Estudantes: ${ok} ✓ ${err} ✗`, ok > 0 ? "ok" : "err");
    await sleep(5000);
    await refreshData();
  };

  const gerarNotas = async () => {
    if (!academia) return;
    if (!academia.ano_letivo) { addLog("Academia sem ano letivo configurado", "err"); return; }
    if (materias.length === 0 || estudantes.length === 0) { addLog("Sem matérias ou estudantes", "warn"); return; }
    const sample = estudantes.slice(0, notaConfig.qtdEstudantes);
    addLog(`Gerando notas para ${sample.length} estudante(s)...`, "step");
    const tipoNota = academia.tipo === "superior" ? "superior" : "escolar";
    const periodosFallback = academia.tipo === "superior" ? ["1_semestre"] : ["1_trimestre","2_trimestre","3_trimestre"];
    const batch: any[] = [];
    for (const est of sample) {
      for (const mat of pickN(materias, Math.min(3, materias.length))) {
        const perds = academia.tipo === "superior" ? (mat.periodo ? [mat.periodo] : periodosFallback) : periodosFallback;
        for (const p of perds) {
          batch.push({ codigo_estudante: est.codigo_estudante, periodo: p, materia_disciplinar_id: mat.id, tipo: tipoNota, categoria: tipoNota === "escolar" ? "nota_escola" : "nota_exame", nota: parseFloat((rnd(8, 20) + Math.random()).toFixed(1)) });
        }
      }
    }
    let ok = 0, err = 0;
    for (let i = 0; i < batch.length; i += 100) {
      if (cancelRef.current) break;
      const { ok: rOk, data } = await callApi("POST", "/academia/notas-aluno/batch", batch.slice(i, i + 100));
      if (rOk) { const items = (data as any)?.items || []; ok += items.filter((x: any) => x.sucesso).length; err += items.filter((x: any) => !x.sucesso).length; }
      else { err += Math.min(100, batch.length - i); }
      await sleep(300);
    }
    addLog(`Notas: ${ok} ✓ ${err} ✗`, ok > 0 ? "ok" : "err");
  };

  const gerarFaltas = async () => {
    if (!academia) return;
    if (!academia.ano_letivo) { addLog("Academia sem ano letivo configurado", "err"); return; }
    if (materias.length === 0 || estudantes.length === 0) { addLog("Sem matérias ou estudantes", "warn"); return; }
    const sample = estudantes.slice(0, faltaConfig.qtdEstudantes);
    addLog(`Gerando faltas para ${sample.length} estudante(s)...`, "step");
    const batch: any[] = [];
    for (const est of sample) {
      for (const mat of pickN(materias, Math.min(2, materias.length))) {
        batch.push({ codigo_estudante: est.codigo_estudante, data: pick(DATAS_FALTA), materia_disciplinar_id: mat.id, quantidade: rnd(1, 3) });
      }
    }
    let ok = 0, err = 0;
    for (let i = 0; i < batch.length; i += 100) {
      if (cancelRef.current) break;
      const { ok: rOk, data } = await callApi("POST", "/academia/faltas-aluno/batch", batch.slice(i, i + 100));
      if (rOk) { const items = (data as any)?.items || []; ok += items.filter((x: any) => x.sucesso).length; err += items.filter((x: any) => !x.sucesso).length; }
      else { err += Math.min(100, batch.length - i); }
      await sleep(300);
    }
    addLog(`Faltas: ${ok} ✓ ${err} ✗`, ok > 0 ? "ok" : "err");
  };

  const gerarAvaliacoes = async () => {
    if (!academia) return;
    if (!academia.ano_letivo) { addLog("Academia sem ano letivo configurado", "err"); return; }
    if (estudantes.length === 0) { addLog("Sem estudantes", "warn"); return; }
    const { tipoEnsino, aprovPct, qtdEstudantes } = avalConfig;
    const sample = estudantes.slice(0, qtdEstudantes);
    const nAprov = Math.floor(sample.length * aprovPct / 100);
    addLog(`Gerando avaliações (${tipoEnsino}) para ${sample.length} estudante(s)...`, "step");
    const batch: any[] = sample.map((est, idx) => {
      const aprovado = idx < nAprov;
      let nivelAtual = "", proximoNivel: string | undefined;
      if (tipoEnsino === "fundamental") {
        const anos = academia.anos_academicos?.filter(a => a.includes("fundamental")) || ["1_ano_fundamental"];
        const i = Math.floor(Math.random() * anos.length);
        nivelAtual = anos[i]; if (aprovado && i < anos.length - 1) proximoNivel = anos[i + 1];
      } else if (tipoEnsino === "medio") {
        const anos = cursos.find(x => x.type === "medio")?.anos_academicos || ["1_ano_medio"];
        nivelAtual = anos[0]; if (aprovado && anos.length > 1) proximoNivel = anos[1];
      } else {
        const anos = cursos.find(x => x.type === "superior")?.anos_academicos || ["1_ano_superior"];
        nivelAtual = anos[0]; if (aprovado && anos.length > 1) proximoNivel = anos[1];
      }
      const p: any = { codigo_estudante: est.codigo_estudante, tipo_ensino: tipoEnsino, nivel_ano_academico_atual: nivelAtual, aprovado, observacao: "Gerado pelo painel de testes" };
      if (aprovado && proximoNivel) p.proximo_ano_academico = proximoNivel;
      return p;
    });
    let ok = 0, err = 0;
    for (let i = 0; i < batch.length; i += 50) {
      if (cancelRef.current) break;
      const { ok: rOk, data } = await callApi("POST", "/academia/avaliacao-final/batch", batch.slice(i, i + 50));
      if (rOk) { const items = (data as any)?.items || []; ok += items.filter((x: any) => x.sucesso).length; err += items.filter((x: any) => !x.sucesso).length; }
      else { err += Math.min(50, batch.length - i); }
      await sleep(400);
    }
    addLog(`Avaliações: ${ok} ✓ ${err} ✗ (${nAprov} aprovações)`, ok > 0 ? "ok" : "err");
  };

  const configurarAnoLetivo = async () => {
    if (!academia) return;
    const ano = "2025_2026";
    const tipo = academia.tipo === "superior" ? "superior" : "escola";
    const { ok, data } = await callApi("POST", "/academia/ano-letivo", { ano_letivo: ano, tipo });
    if (ok) { addLog(`Ano letivo ${ano} configurado ✓`, "ok"); setAcademia(prev => prev ? { ...prev, ano_letivo: ano } : prev); }
    else addLog(`Ano letivo: ${(data as any)?.message || (data as any)?.error}`, "warn");
  };

  // ─── Render helpers ───────────────────────────────────────────────────────────

  const logColors: Record<LogLevel, string> = { ok: "#4ade80", err: "#f87171", warn: "#facc15", info: "#94a3b8", step: "#60a5fa", dim: "#475569" };
  const logIcons: Record<LogLevel, string> = { ok: "✓", err: "✗", warn: "!", info: "·", step: "▶", dim: "·" };

  const Section = ({ title, children, badge }: { title: string; children: React.ReactNode; badge?: string }) => (
    <div style={{ border: "1px solid #1e293b", borderRadius: 12, padding: 20, background: "#0f172a", marginBottom: 16 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 16 }}>
        <h3 style={{ margin: 0, fontSize: 14, fontWeight: 700, color: "#e2e8f0", letterSpacing: "0.05em", textTransform: "uppercase" }}>{title}</h3>
        {badge && <span style={{ fontSize: 11, padding: "2px 8px", background: "#1e3a5f", color: "#60a5fa", borderRadius: 20, fontWeight: 600 }}>{badge}</span>}
      </div>
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

  const Sel = (props: React.SelectHTMLAttributes<HTMLSelectElement>) => (
    <select {...props} style={{ background: "#1e293b", border: "1px solid #334155", color: "#e2e8f0", borderRadius: 8, padding: "6px 10px", fontSize: 13, cursor: "pointer" }} />
  );

  const Inp = (props: React.InputHTMLAttributes<HTMLInputElement>) => (
    <input {...props} style={{ background: "#1e293b", border: "1px solid #334155", color: "#e2e8f0", borderRadius: 8, padding: "6px 10px", fontSize: 13, width: props.type === "number" ? 80 : undefined }} />
  );

  const Btn = ({ onClick, children, color = "#2563eb", disabled = false }: { onClick: () => void; children: React.ReactNode; color?: string; disabled?: boolean }) => (
    <button onClick={onClick} disabled={disabled || running} style={{ background: disabled || running ? "#1e293b" : color, color: disabled || running ? "#475569" : "#fff", border: "none", borderRadius: 8, padding: "8px 18px", fontSize: 13, fontWeight: 600, cursor: disabled || running ? "not-allowed" : "pointer" }}>
      {children}
    </button>
  );

  const anosDisp = academia?.anos_academicos?.filter(a => a.includes("fundamental")) || [];

  // ─── Loading / error ─────────────────────────────────────────────────────────

  if (loadingPerfil) {
    return (
      <div style={{ minHeight: "100vh", background: "#020817", display: "flex", alignItems: "center", justifyContent: "center", color: "#64748b", fontFamily: "monospace" }}>
        Carregando...
      </div>
    );
  }

  if (!academia) {
    return (
      <div style={{ minHeight: "100vh", background: "#020817", display: "flex", alignItems: "center", justifyContent: "center", color: "#f87171", fontFamily: "monospace" }}>
        Não foi possível carregar o perfil da academia.
      </div>
    );
  }

  // ─── Render ───────────────────────────────────────────────────────────────────

  return (
    <div style={{ minHeight: "100vh", background: "#020817", color: "#e2e8f0", padding: 24, fontFamily: "'JetBrains Mono', 'Fira Code', 'Courier New', monospace" }}>
      <div style={{ maxWidth: 1200, margin: "0 auto" }}>

        <div style={{ marginBottom: 28 }}>
          <h1 style={{ margin: 0, fontSize: 22, fontWeight: 800, color: "#f8fafc", letterSpacing: "-0.02em" }}>
            <span style={{ color: "#3b82f6" }}>⬡</span> Painel de Testes
          </h1>
          <p style={{ margin: "6px 0 0", fontSize: 13, color: "#64748b" }}>
            {academia.nome} · {academia.codigo} · {academia.tipo}{academia.nivel ? ` · ${academia.nivel}` : ""}{academia.ano_letivo ? ` · ${academia.ano_letivo.replace("_","/")}` : " · sem ano letivo"}
          </p>
        </div>

        <div style={{ display: "grid", gridTemplateColumns: "260px 1fr", gap: 20, alignItems: "start" }}>

          {/* LEFT */}
          <div>
            <Section title="Estado atual">
              {[
                { label: "Cursos", val: cursos.length, icon: "📚" },
                { label: "Matérias ativas", val: materias.length, icon: "📖" },
                { label: "Turmas", val: turmas.length, icon: "🏫" },
                { label: "Estudantes", val: estudantes.length, icon: "👥" },
              ].map(s => (
                <div key={s.label} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "8px 0", borderBottom: "1px solid #1e293b" }}>
                  <span style={{ fontSize: 13, color: "#94a3b8" }}>{s.icon} {s.label}</span>
                  <span style={{ fontSize: 16, fontWeight: 800, color: s.val > 0 ? "#4ade80" : "#ef4444" }}>{s.val}</span>
                </div>
              ))}
              <div style={{ marginTop: 14, display: "flex", flexDirection: "column", gap: 8 }}>
                <Btn onClick={() => withLoading(configurarAnoLetivo)} color="#0f4c75">
                  {academia.ano_letivo ? `✓ Ano ${academia.ano_letivo.replace("_","/")}` : "Configurar ano letivo"}
                </Btn>
                <Btn onClick={refreshData} color="#334155">↻ Atualizar</Btn>
              </div>
            </Section>
          </div>

          {/* RIGHT */}
          <div>
            {academia.nivel !== "fundamental" && (
              <Section title="Cursos" badge={`${cursos.length} criados`}>
                <Row>
                  <Field label="Tipo">
                    <Sel value={cursoConfig.tipo} onChange={e => setCursoConfig(p => ({ ...p, tipo: e.target.value as any }))}>
                      {academia.tipo === "superior" ? (
                        <option value="superior">Superior</option>
                      ) : (<>
                        <option value="medio">Médio</option>
                        <option value="superior">Superior</option>
                      </>)}
                    </Sel>
                  </Field>
                  <Field label="Quantidade">
                    <Inp type="number" min={1} max={5} value={cursoConfig.qtd} onChange={e => setCursoConfig(p => ({ ...p, qtd: +e.target.value }))} />
                  </Field>
                  <Btn onClick={() => withLoading(gerarCursos)} color="#7c3aed">Gerar Cursos</Btn>
                </Row>
                {cursos.length > 0 && (
                  <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                    {cursos.map(c => <span key={c.id} style={{ fontSize: 11, padding: "3px 8px", background: "#1e293b", borderRadius: 20, color: "#94a3b8" }}>{c.nome}</span>)}
                  </div>
                )}
              </Section>
            )}

            <Section title="Matérias" badge={`${materias.length} ativas`}>
              <Row>
                <Field label="Tipo">
                  <Sel value={materiaConfig.tipo} onChange={e => setMateriaConfig(p => ({ ...p, tipo: e.target.value as any }))}>
                    {(academia.nivel === "fundamental" || academia.nivel === "misto") && <option value="fundamental">Fundamental</option>}
                    {(academia.nivel === "medio" || academia.nivel === "misto") && <option value="medio">Médio</option>}
                    {academia.tipo === "superior" && <option value="superior">Superior</option>}
                  </Sel>
                </Field>
                {(materiaConfig.tipo === "medio" || materiaConfig.tipo === "superior") && (
                  <Field label="Curso">
                    <Sel value={materiaConfig.cursoId} onChange={e => setMateriaConfig(p => ({ ...p, cursoId: e.target.value }))}>
                      <option value="auto">Auto</option>
                      {cursos.filter(c => c.type === materiaConfig.tipo).map(c => <option key={c.id} value={c.id}>{c.nome}</option>)}
                    </Sel>
                  </Field>
                )}
                <Field label="Quantidade">
                  <Inp type="number" min={1} max={10} value={materiaConfig.qtd} onChange={e => setMateriaConfig(p => ({ ...p, qtd: +e.target.value }))} />
                </Field>
                <Btn onClick={() => withLoading(gerarMaterias)} color="#7c3aed">Gerar Matérias</Btn>
              </Row>
              {materias.length > 0 && (
                <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
                  {materias.slice(0, 10).map(m => <span key={m.id} style={{ fontSize: 11, padding: "3px 8px", background: "#1e293b", borderRadius: 20, color: "#94a3b8" }}>{m.nome}</span>)}
                  {materias.length > 10 && <span style={{ fontSize: 11, color: "#475569" }}>+{materias.length - 10} mais</span>}
                </div>
              )}
            </Section>

            <Section title="Turmas" badge={`${turmas.length} criadas`}>
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
                <Field label="Nível">
                  <Sel value={turmaConfig.nivel} onChange={e => setTurmaConfig(p => ({ ...p, nivel: e.target.value }))}>
                    <option value="random">Aleatório</option>
                    {anosDisp.map(a => <option key={a} value={a}>{a.replace("_ano_fundamental","º Fund.")}</option>)}
                    {cursos.filter(c => c.type === "medio").flatMap(c => c.anos_academicos).map(a => <option key={a} value={a}>{a.replace("_ano_medio","º Médio")}</option>)}
                    {cursos.filter(c => c.type === "superior").flatMap(c => c.anos_academicos).map(a => <option key={a} value={a}>{a.replace("_ano_superior","º Superior")}</option>)}
                  </Sel>
                </Field>
                <Btn onClick={() => withLoading(gerarTurmas)} color="#0891b2">Gerar Turmas</Btn>
                {turmas.length > 0 && estudantes.length > 0 && (
                  <Btn onClick={() => withLoading(vincularEstudantesATurmas)} color="#0f4c75">Vincular Estudantes</Btn>
                )}
              </Row>
            </Section>

            <Section title="Estudantes" badge={`${estudantes.length} cadastrados`}>
              <Row>
                <Field label="Quantidade">
                  <Inp type="number" min={1} max={500} value={estudanteConfig.qtd} onChange={e => setEstudanteConfig(p => ({ ...p, qtd: +e.target.value }))} />
                </Field>
                {(academia.nivel === "fundamental" || academia.nivel === "misto") && (
                  <Field label="Ano escolar">
                    <Sel value={estudanteConfig.anoEscolar} onChange={e => setEstudanteConfig(p => ({ ...p, anoEscolar: e.target.value }))}>
                      <option value="random">Aleatório</option>
                      {anosDisp.map(a => <option key={a} value={a}>{a.replace("_ano_fundamental","º Fund.")}</option>)}
                    </Sel>
                  </Field>
                )}
                <Field label="Status">
                  <Sel value={estudanteConfig.statusFund} onChange={e => setEstudanteConfig(p => ({ ...p, statusFund: e.target.value }))}>
                    <option value="em_andamento">Em andamento</option>
                    <option value="inativo">Inativo</option>
                    <option value="finalizado">Finalizado</option>
                  </Sel>
                </Field>
                <Btn onClick={() => withLoading(gerarEstudantes)} color="#059669">Gerar Estudantes</Btn>
              </Row>
            </Section>

            <Section title="Notas" badge={materias.length === 0 ? "crie matérias primeiro" : undefined}>
              <Row>
                <Field label="Nº estudantes">
                  <Inp type="number" min={1} max={estudantes.length || 100} value={notaConfig.qtdEstudantes} onChange={e => setNotaConfig(p => ({ ...p, qtdEstudantes: +e.target.value }))} />
                </Field>
                <Btn onClick={() => withLoading(gerarNotas)} color="#b45309" disabled={materias.length === 0 || estudantes.length === 0}>Gerar Notas</Btn>
              </Row>
              <p style={{ margin: 0, fontSize: 11, color: "#475569" }}>
                {academia.tipo === "superior" ? "tipo: superior · categoria: nota_exame · período: por matéria" : "tipo: escolar · categoria: nota_escola · 3 trimestres"}
              </p>
            </Section>

            <Section title="Faltas" badge={materias.length === 0 ? "crie matérias primeiro" : undefined}>
              <Row>
                <Field label="Nº estudantes">
                  <Inp type="number" min={1} max={estudantes.length || 100} value={faltaConfig.qtdEstudantes} onChange={e => setFaltaConfig(p => ({ ...p, qtdEstudantes: +e.target.value }))} />
                </Field>
                <Btn onClick={() => withLoading(gerarFaltas)} color="#b45309" disabled={materias.length === 0 || estudantes.length === 0}>Gerar Faltas</Btn>
              </Row>
              <p style={{ margin: 0, fontSize: 11, color: "#475569" }}>2 matérias por estudante · datas aleatórias de 2025</p>
            </Section>

            <Section title="Avaliações Finais" badge={estudantes.length === 0 ? "crie estudantes primeiro" : undefined}>
              <Row>
                <Field label="Tipo ensino">
                  <Sel value={avalConfig.tipoEnsino} onChange={e => setAvalConfig(p => ({ ...p, tipoEnsino: e.target.value }))}>
                    {(academia.nivel === "fundamental" || academia.nivel === "misto") && <option value="fundamental">Fundamental</option>}
                    {(academia.nivel === "medio" || academia.nivel === "misto") && <option value="medio">Médio</option>}
                    {academia.tipo === "superior" && <option value="superior">Superior</option>}
                  </Sel>
                </Field>
                <Field label="% Aprovação">
                  <Inp type="number" min={0} max={100} value={avalConfig.aprovPct} onChange={e => setAvalConfig(p => ({ ...p, aprovPct: +e.target.value }))} />
                </Field>
                <Field label="Nº estudantes">
                  <Inp type="number" min={1} max={estudantes.length || 100} value={avalConfig.qtdEstudantes} onChange={e => setAvalConfig(p => ({ ...p, qtdEstudantes: +e.target.value }))} />
                </Field>
                <Btn onClick={() => withLoading(gerarAvaliacoes)} color="#7c3aed" disabled={estudantes.length === 0}>Gerar Avaliações</Btn>
              </Row>
            </Section>
          </div>
        </div>

        {/* Log terminal */}
        {logs.length > 0 && (
          <div style={{ marginTop: 20, border: "1px solid #1e293b", borderRadius: 12, overflow: "hidden" }}>
            <div style={{ background: "#0f172a", padding: "10px 16px", display: "flex", justifyContent: "space-between", alignItems: "center", borderBottom: "1px solid #1e293b" }}>
              <span style={{ fontSize: 12, color: "#475569", fontWeight: 600 }}>LOG · {logs.length} entradas</span>
              <div style={{ display: "flex", gap: 10 }}>
                {running && (
                  <button onClick={() => { cancelRef.current = true; addLog("Cancelando...", "warn"); }} style={{ background: "#7f1d1d", color: "#fca5a5", border: "none", borderRadius: 6, padding: "4px 12px", fontSize: 12, cursor: "pointer" }}>✕ Cancelar</button>
                )}
                <button onClick={() => setLogs([])} style={{ background: "transparent", color: "#475569", border: "1px solid #1e293b", borderRadius: 6, padding: "4px 12px", fontSize: 12, cursor: "pointer" }}>Limpar</button>
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