---
criado: 2026-08-19
origem: Claude (orquestrador) — a pedido de Fredy Luís, Fundador e CEO da Spuri
status: concluída
tarefa: 49b — Correções adicionais das páginas /financas/* (subtelas, terminologia, backend pendente)
---

# Tarefa 49b — Correções adicionais das páginas `/financas/*`

## Prompt recomendado para o Codex

> Este é um documento de **continuação** da tarefa 49. Execute exatamente o que está descrito aqui, arquivo por arquivo, na ordem em que aparecem. Não planeje nada — tudo já foi decidido e validado (backend: `go build`, `go vet`, `gofmt` e teste de integração real com PostgreSQL 16; frontend: `tsc --noEmit`, `eslint` e `next build`, todos sem erro). Onde o documento manda **substituir o arquivo inteiro**, apague o conteúdo atual e cole exatamente o bloco fornecido. Onde dá um bloco "SUBSTITUIR" com texto antes/depois, localize o texto exato (`old_str`) e troque só por aquele trecho — se não bater 100%, pare e reporte a diferença. **Importante: a Seção 1 deste documento é no repositório `spuri-backend`, não em `spuripainel`** — confirme em qual repositório você está antes de aplicar cada seção (o título de cada seção indica o repositório). Ao final, rode as validações da seção "Procedimento de conclusão" e cole o resultado bruto do terminal na resposta.

## Contexto

Duas coisas motivam este documento:

1. **A extensão de backend da tarefa 49 (filtro `tipo` no endpoint de cobranças do estudante) nunca foi aplicada.** O relatório da execução anterior mencionou só arquivos do `spuripainel` — o commit `391be7c` (PR #280, já mesclado em `main`) confirma isso: `spuri-backend` está exatamente como estava antes da tarefa 49, sem `origensClause` nem o parâmetro `origens` em `ListCobrancasEstudante`. Isso significa que o filtro "Tipo de cobrança" na tela do estudante (`/pagamentos`, seção "Histórico completo de cobranças") está sendo enviado pelo frontend mas **silenciosamente ignorado pelo backend** — o estudante seleciona um tipo e continua vendo todos. A Seção 1 abaixo corrige isso.
2. **Feedback de revisão sobre a tarefa 49**, com 8 pontos novos, todos já investigados e com o código correspondente já escrito e validado (Seções 2-6): terminologia do ensino fundamental, inferência automática de nível/curso/ano a partir dos dados da própria academia, formatação de ano letivo, separação da seção de anular/reativar obrigações, divisão da página de configurações em subtelas (em vez de seções empilhadas), proibição de checkbox nativo em qualquer lugar, uso de subtelas em vez de pop-ups/modais, e remoção do botão de cancelar cobrança de dentro do detalhe da cobrança.

---

## Resumo executivo

| # | Arquivo | Ação | Repositório |
|---|---|---|---|
| 1 | `internal/finance/appypay.go` | Editar (blocos precisos) | **spuri-backend** |
| 2 | `internal/handlers/financeiro_handlers.go` | Editar (1 linha) | **spuri-backend** |
| 3 | `internal/finance/cobrancas_estudante_integration_test.go` | Editar (blocos precisos) | **spuri-backend** |
| 4 | `src/components/paineis/financeiroShared.tsx` | Substituir arquivo inteiro | spuripainel |
| 5 | `src/components/paineis/FinanceiroConfiguracoesPainel.tsx` | Substituir arquivo inteiro | spuripainel |
| 6 | `src/components/paineis/AnularReativarObrigacoesForm.tsx` | Substituir arquivo inteiro | spuripainel |
| 7 | `src/components/paineis/FinanceiroPagamentosPainel.tsx` | Substituir arquivo inteiro | spuripainel |
| 8 | `src/components/paineis/EstudantePagamentosPainel.tsx` | Substituir arquivo inteiro | spuripainel |

**`FinanceiroCredenciaisPainel.tsx` não muda nesta rodada** — já não usa checkbox nativo, já não usa modal (usa o mesmo padrão de subtela que está sendo estendido agora às outras páginas), e não exibe nenhum rótulo de nível de ensino. Nenhum arquivo deve ser removido.

---

## Seção 1 — `spuri-backend/internal/finance/appypay.go` (editar)

**Objetivo:** extrair a lógica de filtro por tipo de cobrança (`origensClause`, hoje só dentro de `ListCobrancas`) para uma função compartilhada, e usá-la também em `ListCobrancasEstudante` — necessário para o estudante conseguir filtrar por tipo em `/pagamentos`.

### 1.1 — Extrair `origensClause` de dentro de `ListCobrancas` e usá-la ali

SUBSTITUIR:
```go
	if len(origens) > 0 {
		clauses := make([]string, 0, len(origens))
		for _, origem := range origens {
			switch origem {
			case "matricula":
				clauses = append(clauses, "COALESCE(payload->>'codigo_solicitacao','') <> ''")
			case "mensalidade":
				clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_estudante','') <> '')")
			case "avulsa":
				clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_estudante','') = '')")
			default:
				return nil, fmt.Errorf("tipo de cobrança inválido: %s", origem)
			}
		}
		where += " AND (" + strings.Join(clauses, " OR ") + ")"
	}
	var total int
	if err := s.client.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM financeiro_cobrancas "+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`SELECT id, COALESCE(provider_charge_id,''), merchant_transaction_id, contexto_tipo, COALESCE(codigo_academia,''), payload, updated_at FROM financeiro_cobrancas %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, where, i, i+1)
	args = append(args, limit, offset)
	rows, err := s.client.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CobrancaResumo{}
	for rows.Next() {
		dto, err := scanCobrancaResumo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
}

// scanCobrancaResumo lê uma linha de financeiro_cobrancas (id,
```

POR:
```go
	if len(origens) > 0 {
		clause, err := origensClause(origens)
		if err != nil {
			return nil, err
		}
		where += clause
	}
	var total int
	if err := s.client.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM financeiro_cobrancas "+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	q := fmt.Sprintf(`SELECT id, COALESCE(provider_charge_id,''), merchant_transaction_id, contexto_tipo, COALESCE(codigo_academia,''), payload, updated_at FROM financeiro_cobrancas %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, where, i, i+1)
	args = append(args, limit, offset)
	rows, err := s.client.DB().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CobrancaResumo{}
	for rows.Next() {
		dto, err := scanCobrancaResumo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &CobrancaListResult{Cobrancas: out, Total: total}, nil
}

// origensClause monta a cláusula SQL "AND (...)" que filtra
// financeiro_cobrancas pelo tipo de cobrança derivado do payload
// (mensalidade, matrícula ou avulsa) — a mesma derivação usada por
// scanCobrancaResumo. Devolve "" (sem filtro) quando origens está vazio.
// Extraída durante a tarefa 49 para ser compartilhada por ListCobrancas e
// ListCobrancasEstudante e nunca divergir entre as duas.
func origensClause(origens []string) (string, error) {
	if len(origens) == 0 {
		return "", nil
	}
	clauses := make([]string, 0, len(origens))
	for _, origem := range origens {
		switch origem {
		case "matricula":
			clauses = append(clauses, "COALESCE(payload->>'codigo_solicitacao','') <> ''")
		case "mensalidade":
			clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_estudante','') <> '')")
		case "avulsa":
			clauses = append(clauses, "(COALESCE(payload->>'codigo_solicitacao','') = '' AND COALESCE(payload->>'codigo_estudante','') = '')")
		default:
			return "", fmt.Errorf("tipo de cobrança inválido: %s", origem)
		}
	}
	return " AND (" + strings.Join(clauses, " OR ") + ")", nil
}

// scanCobrancaResumo lê uma linha de financeiro_cobrancas (id,
```

### 1.2 — Assinatura de `ListCobrancasEstudante` (novo parâmetro `origens`)

SUBSTITUIR:
```go
func (s *Service) ListCobrancasEstudante(ctx context.Context, codigoEstudante string, somenteAcademia *string, estados []string, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	if codigoEstudante == "" {
		return nil, errors.New("código do estudante é obrigatório")
	}
	where := `WHERE (payload->>'codigo_estudante' = $1 OR payload->>'codigo_solicitacao' IN (SELECT codigo_solicitacao FROM projection_solicitacoes_matricula WHERE codigo_estudante_gerado = $1))`
	args := []any{codigoEstudante}
	i := 2
	if somenteAcademia != nil {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, *somenteAcademia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estados))
		i++
	}
	var total int
```

POR:
```go
func (s *Service) ListCobrancasEstudante(ctx context.Context, codigoEstudante string, somenteAcademia *string, estados, origens []string, limit, offset int) (*CobrancaListResult, error) {
	if s.client == nil {
		return nil, errors.New("serviço financeiro não inicializado")
	}
	if codigoEstudante == "" {
		return nil, errors.New("código do estudante é obrigatório")
	}
	where := `WHERE (payload->>'codigo_estudante' = $1 OR payload->>'codigo_solicitacao' IN (SELECT codigo_solicitacao FROM projection_solicitacoes_matricula WHERE codigo_estudante_gerado = $1))`
	args := []any{codigoEstudante}
	i := 2
	if somenteAcademia != nil {
		where += fmt.Sprintf(" AND codigo_academia=$%d", i)
		args = append(args, *somenteAcademia)
		i++
	}
	if len(estados) > 0 {
		where += fmt.Sprintf(" AND payload->>'status' = ANY($%d)", i)
		args = append(args, pq.Array(estados))
		i++
	}
	if len(origens) > 0 {
		clause, err := origensClause(origens)
		if err != nil {
			return nil, err
		}
		where += clause
	}
	var total int
```

**Atenção:** `ListCobrancasEstudante` é chamada em 5 lugares — 1 no handler (Seção 2) e 4 em testes de integração (Seção 3). Todos precisam do parâmetro `origens` adicionado na posição correta (depois de `estados`, antes de `limit`).

---

## Seção 2 — `spuri-backend/internal/handlers/financeiro_handlers.go` (editar 1 linha)

SUBSTITUIR:
```go
	res, err := FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, c.QueryArray("estado"), limit, offset)
```

POR:
```go
	res, err := FinanceiroService.ListCobrancasEstudante(c.Request.Context(), codigo, somenteAcademia, c.QueryArray("estado"), c.QueryArray("tipo"), limit, offset)
```

---

## Seção 3 — `spuri-backend/internal/finance/cobrancas_estudante_integration_test.go` (editar)

**Objetivo:** atualizar as 4 chamadas existentes a `ListCobrancasEstudante` com o novo parâmetro, e adicionar 4 casos novos cobrindo o filtro por tipo.

### 3.1 — dentro de `TestIntegrationListCobrancasEstudanteIncluiMensalidadeEMatricula`

SUBSTITUIR:
```go
	res, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 {
		t.Fatalf("esperava 3 cobranças do estudante (1 matrícula + 2 mensalidade), obteve %d: %#v", res.Total, res.Cobrancas)
	}
	var temMatricula, temFalhada bool
	for _, cobranca := range res.Cobrancas {
		if cobranca.Origem == "matricula" && cobranca.CodigoSolicitacao == codigoSolicitacao {
			temMatricula = true
		}
		if cobranca.Status == "Failed" {
			temFalhada = true
		}
	}
	if !temMatricula {
		t.Fatalf("cobrança de matrícula original não apareceu na listagem: %#v", res.Cobrancas)
	}
	if !temFalhada {
		t.Fatalf("cobrança falhada não apareceu (listagem deveria incluir todos os estados por padrão): %#v", res.Cobrancas)
	}

	pagas, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, []string{"Success"}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if pagas.Total != 2 {
		t.Fatalf("filtro por estado=Success deveria devolver 2 cobranças (matrícula + mensalidade paga), obteve %d", pagas.Total)
	}
}
```

POR:
```go
	res, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 {
		t.Fatalf("esperava 3 cobranças do estudante (1 matrícula + 2 mensalidade), obteve %d: %#v", res.Total, res.Cobrancas)
	}
	var temMatricula, temFalhada bool
	for _, cobranca := range res.Cobrancas {
		if cobranca.Origem == "matricula" && cobranca.CodigoSolicitacao == codigoSolicitacao {
			temMatricula = true
		}
		if cobranca.Status == "Failed" {
			temFalhada = true
		}
	}
	if !temMatricula {
		t.Fatalf("cobrança de matrícula original não apareceu na listagem: %#v", res.Cobrancas)
	}
	if !temFalhada {
		t.Fatalf("cobrança falhada não apareceu (listagem deveria incluir todos os estados por padrão): %#v", res.Cobrancas)
	}

	pagas, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, []string{"Success"}, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if pagas.Total != 2 {
		t.Fatalf("filtro por estado=Success deveria devolver 2 cobranças (matrícula + mensalidade paga), obteve %d", pagas.Total)
	}

	// tarefa 49: o próprio estudante também precisa conseguir filtrar por
	// tipo de cobrança (mensalidade/matrícula/avulsa), mesmo mecanismo que
	// ListCobrancas já oferece à academia/admin.
	somenteMensalidade, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"mensalidade"}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if somenteMensalidade.Total != 2 {
		t.Fatalf("filtro por tipo=mensalidade deveria devolver 2 cobranças, obteve %d: %#v", somenteMensalidade.Total, somenteMensalidade.Cobrancas)
	}
	for _, cobranca := range somenteMensalidade.Cobrancas {
		if cobranca.Origem != "mensalidade" {
			t.Fatalf("filtro por tipo=mensalidade devolveu cobrança de origem %q: %#v", cobranca.Origem, cobranca)
		}
	}

	somenteMatricula, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"matricula"}, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if somenteMatricula.Total != 1 || somenteMatricula.Cobrancas[0].CodigoSolicitacao != codigoSolicitacao {
		t.Fatalf("filtro por tipo=matricula deveria devolver só a cobrança da matrícula original, obteve %#v", somenteMatricula.Cobrancas)
	}

	if _, err := service.ListCobrancasEstudante(ctx, codigoEstudante, nil, nil, []string{"invalido"}, 50, 0); err == nil {
		t.Fatal("esperava erro para tipo de cobrança inválido")
	}
```

### 3.2 — dentro de `TestIntegrationListCobrancasEstudanteSomenteAcademiaIsolaOutraAcademia`

SUBSTITUIR:
```go
	semRestricao, err := service.ListCobrancasEstudante(ctx, estudante, nil, nil, 50, 0)
```

POR:
```go
	semRestricao, err := service.ListCobrancasEstudante(ctx, estudante, nil, nil, nil, 50, 0)
```

### 3.3 — mesma função, chamada seguinte

SUBSTITUIR:
```go
	comRestricao, err := service.ListCobrancasEstudante(ctx, estudante, &academiaA, nil, 50, 0)
```

POR:
```go
	comRestricao, err := service.ListCobrancasEstudante(ctx, estudante, &academiaA, nil, nil, 50, 0)
```

**Validação já feita por Claude:** `go build ./...`, `go vet ./...` e `gofmt -l` sem nenhum erro/diferença nos 3 arquivos desta rodada. Rodei `go test ./internal/finance/... -run TestIntegrationListCobrancasEstudante -v` com PostgreSQL 16 real: **os dois testes passam**, incluindo os 4 novos casos do filtro por tipo.

---

## Seção 4 — `spuripainel/src/components/paineis/financeiroShared.tsx` (substituir arquivo inteiro)

**Objetivo (pontos 6, 7 e 8 do feedback):**
- Adiciona `formatAnoLetivo` ("2026_2027" → "2026/2027" — nunca mais exibir o valor cru), `NIVEL_LABEL` (terminologia correta: "fundamental" exibido como "Ensino Primário e Iº Ciclo", igual ao resto do painel) e `niveisDaAcademia` (infere quais níveis a academia realmente oferece a partir de `academia.nivel`/`academia.nivel_escolar`, a mesma regra já usada em `MateriaPainel.tsx`/`TurmasPainel.tsx` — nunca uma lista fixa fundamental/médio/superior).
- Adiciona `SubtelaPanel`, `SubtelaCard` e `SubtelasMenu`: o padrão de "subtela" (título + botão Voltar, sem sobreposição) generalizado para ser reutilizado em todas as páginas financeiras — o mesmo padrão que `FinanceiroCredenciaisPainel.tsx` já usava para criar/editar credencial, agora disponível para qualquer outra tela do módulo.
- **`CobrancaDetalhesModal` (que usava `<Modal>`) foi substituído por `SubtelaDetalheCobranca`** — mesmo conteúdo, mas renderizado como subtela (via `SubtelaPanel`), sem pop-up. **E não tem mais botão de cancelar** — cancelar deixou de ser uma ação dentro do detalhe.
- `CobrancasTable` ganhou um parâmetro opcional `onCancelar`: quando fornecido, cada linha cancelável mostra um botão "Cancelar" independente do "Ver detalhes" — a ação de cancelar fica no seu contexto (a linha da cobrança na lista), não misturada com a leitura dos detalhes.

SUBSTITUIR O ARQUIVO INTEIRO por:

```tsx
"use client";
import { useEffect, useState } from "react";
import { consultasService } from "@/lib/api";
import { formatApiError } from "@/lib/api/client";
import Button from "@/components/ui/button/Button";
import Icon from "@/components/ui/Icon";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "@/components/ui/table";
import type { AcademiaNivel, CobrancaResumo, EstudanteDetalhado, FinanceiroNivel, FinanceiroOrigemCobranca, NivelEscolar } from "@/types/api";

/**
 * Utilitários e componentes partilhados pelas telas de pagamentos e de
 * configurações financeiras (FinanceiroPagamentosPainel,
 * EstudantePagamentosPainel, FinanceiroConfiguracoesPainel e
 * MatriculaPublicPage). Extraído da tarefa 49 e ampliado na correção
 * seguinte (subtelas em vez de modais, terminologia e inferência de nível
 * a partir dos dados da própria academia).
 */

export const money = (v: number) =>
  new Intl.NumberFormat("pt-AO", { style: "currency", currency: "AOA" }).format(v);

export const dt = (v: string) => {
  const d = new Date(v);
  return Number.isNaN(d.getTime())
    ? v
    : new Intl.DateTimeFormat("pt-AO", { dateStyle: "short", timeStyle: "short" }).format(d);
};

/** "2026_2027" (formato de armazenamento no backend) → "2026/2027" (exibição). Nunca mostrar o valor cru. */
export function formatAnoLetivo(v?: string) {
  if (!v) return "—";
  return v.includes("_") ? v.replace("_", "/") : v;
}

/** "Avulsa" no backend passou a ser apresentada como "Outros" no frontend (pedido do produto). */
export const origemLabel: Record<FinanceiroOrigemCobranca, string> = {
  matricula: "Matrícula",
  mensalidade: "Mensalidade",
  avulsa: "Outros",
};

/**
 * Rótulo de exibição de cada nível de ensino, seguindo a terminologia
 * angolana já padronizada no resto do painel (MateriaPainel.tsx,
 * TurmasPainel.tsx): "fundamental" nunca aparece como "Fundamental" para o
 * usuário, e sim como "Ensino Primário e Iº Ciclo".
 */
export const NIVEL_LABEL: Record<FinanceiroNivel, string> = {
  fundamental: "Ensino Primário e Iº Ciclo",
  medio: "Médio",
  superior: "Superior",
};

/**
 * Infere os níveis de ensino que a academia realmente oferece — nunca uma
 * lista fixa. Mesma regra usada em MateriaPainel.tsx/TurmasPainel.tsx:
 * `academia.nivel === "superior"` → só superior; `academia.nivel ===
 * "escola"` → depende de `academia.nivel_escolar` (fundamental | medio |
 * misto).
 */
export function niveisDaAcademia(academia?: { nivel?: AcademiaNivel; nivel_escolar?: NivelEscolar }): FinanceiroNivel[] {
  if (!academia) return [];
  if (academia.nivel === "superior") return ["superior"];
  if (academia.nivel === "escola") {
    if (academia.nivel_escolar === "fundamental") return ["fundamental"];
    if (academia.nivel_escolar === "medio") return ["medio"];
    if (academia.nivel_escolar === "misto") return ["fundamental", "medio"];
  }
  return [];
}

export function StatusBadge({ status }: { status: string }) {
  const x = status.toLowerCase();
  const cls = x.includes("success") || x.includes("pago")
    ? "bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400"
    : x.includes("pend")
    ? "bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300"
    : x.includes("fail") || x.includes("falh")
    ? "bg-red-100 text-red-800 dark:bg-red-900/20 dark:text-red-400"
    : x.includes("cancel")
    ? "bg-gray-100 text-gray-800 dark:bg-gray-900/20 dark:text-gray-400"
    : "bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400";
  return <span className={`rounded-full px-2 py-1 text-xs font-medium ${cls}`}>{status}</span>;
}

export function Qr({ value }: { value?: string }) {
  if (!value) return null;
  return value.startsWith("data:image") ? (
    // eslint-disable-next-line @next/next/no-img-element
    <img src={value} alt="QR Code" className="max-h-64 rounded border p-2" />
  ) : (
    <pre className="whitespace-pre-wrap rounded bg-gray-50 p-3 text-xs dark:bg-gray-800">{value}</pre>
  );
}

export function LoadingState({ label = "Carregando..." }: { label?: string }) {
  return (
    <div className="flex min-h-[220px] items-center justify-center rounded-2xl border border-gray-200 bg-white dark:border-white/[0.05] dark:bg-white/[0.03]">
      <div className="flex items-center gap-3 text-sm text-gray-500 dark:text-gray-400">
        <span className="h-5 w-5 animate-spin rounded-full border-2 border-gray-300 border-t-brand-500" />
        {label}
      </div>
    </div>
  );
}

export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <div className="rounded-xl border border-dashed border-gray-200 p-8 text-center dark:border-white/[0.08]">
      <Icon icon="mdi:credit-card-off-outline" width={32} className="mx-auto text-gray-400" />
      <p className="mt-2 font-medium text-gray-800 dark:text-white/90">{title}</p>
      <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">{description}</p>
    </div>
  );
}

/**
 * Paginação por botões numerados. Mesmo padrão visual e de comportamento
 * do componente PaginacaoSetas usado em /estudantes (PageContent.tsx) —
 * replicado aqui (e não importado de lá) porque aquele componente não é
 * exportado e /estudantes está fora do escopo desta tarefa.
 */
export function PaginacaoSetas({ paginaAtual, totalPaginas, total, porPagina, onChange }: {
  paginaAtual: number; totalPaginas: number; total: number; porPagina: number; onChange: (p: number) => void;
}) {
  if (totalPaginas <= 1) return null;
  const inicio = (paginaAtual - 1) * porPagina + 1;
  const fim = Math.min(paginaAtual * porPagina, total);
  const getPages = (): (number | "...")[] => {
    const pages: (number | "...")[] = [];
    if (totalPaginas <= 7) { for (let i = 1; i <= totalPaginas; i++) pages.push(i); }
    else if (paginaAtual <= 4) { for (let i = 1; i <= 5; i++) pages.push(i); pages.push("..."); pages.push(totalPaginas); }
    else if (paginaAtual >= totalPaginas - 3) { pages.push(1); pages.push("..."); for (let i = totalPaginas - 4; i <= totalPaginas; i++) pages.push(i); }
    else { pages.push(1); pages.push("..."); for (let i = paginaAtual - 1; i <= paginaAtual + 1; i++) pages.push(i); pages.push("..."); pages.push(totalPaginas); }
    return pages;
  };
  return (
    <div className="flex flex-col sm:flex-row items-center justify-between gap-3 px-4 py-3 bg-white dark:bg-white/[0.03] rounded-lg border border-gray-200 dark:border-white/[0.05]">
      <p className="text-sm text-gray-500 dark:text-gray-400">{inicio}–{fim} de {total}</p>
      <div className="flex items-center gap-1">
        <button onClick={() => onChange(paginaAtual - 1)} disabled={paginaAtual === 1}
          className="p-1.5 rounded-md text-gray-500 hover:bg-gray-100 dark:hover:bg-white/[0.05] disabled:opacity-40 disabled:cursor-not-allowed transition-colors">
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" /></svg>
        </button>
        {getPages().map((p, i) => p === "..." ? (
          <span key={`e${i}`} className="px-1.5 text-gray-400 text-sm select-none">…</span>
        ) : (
          <button key={p} onClick={() => onChange(p as number)}
            className={`min-w-[32px] h-8 rounded-md text-sm font-medium transition-colors ${paginaAtual === p ? "bg-brand-500 text-white" : "text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-white/[0.05]"}`}>{p}</button>
        ))}
        <button onClick={() => onChange(paginaAtual + 1)} disabled={paginaAtual === totalPaginas}
          className="p-1.5 rounded-md text-gray-500 hover:bg-gray-100 dark:hover:bg-white/[0.05] disabled:opacity-40 disabled:cursor-not-allowed transition-colors">
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" /></svg>
        </button>
      </div>
      <p className="text-sm text-gray-500 dark:text-gray-400">Pág. {paginaAtual}/{totalPaginas}</p>
    </div>
  );
}

/**
 * Container padrão de subtela: título + botão "Voltar", sem sobreposição
 * (nada de pop-up/modal). Usado por toda parte do módulo financeiro que
 * precisa abrir uma tela de detalhe/formulário a partir de uma lista ou de
 * um menu de opções — o mesmo padrão já usado em
 * FinanceiroCredenciaisPainel para criar/editar credencial.
 */
export function SubtelaPanel({ title, icon, onVoltar, children }: { title: string; icon?: string; onVoltar: () => void; children: React.ReactNode }) {
  return (
    <div className="space-y-5 rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/[0.05] dark:bg-white/[0.03] lg:p-8">
      <Button variant="outline" size="sm" onClick={onVoltar} startIcon={<Icon icon="mdi:arrow-left" width={16} />}>Voltar</Button>
      <div className="flex items-center gap-2">
        {icon && <Icon icon={icon} width={22} className="text-gray-800 dark:text-white/90" />}
        <h2 className="text-lg font-semibold text-gray-800 dark:text-white/90">{title}</h2>
      </div>
      {children}
    </div>
  );
}

/** Cartão clicável de uma opção do menu de subtelas (ex.: menu de configurações financeiras). */
export function SubtelaCard({ icon, label, descricao, onClick }: { icon: string; label: string; descricao: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-start gap-3 rounded-2xl border border-gray-200 bg-white p-4 text-left transition hover:border-brand-300 hover:shadow-sm dark:border-white/[0.05] dark:bg-white/[0.03] dark:hover:border-brand-500/40"
    >
      <Icon icon={icon} width={22} className="mt-0.5 shrink-0 text-brand-500" />
      <div>
        <p className="font-medium text-gray-800 dark:text-white/90">{label}</p>
        <p className="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{descricao}</p>
      </div>
    </button>
  );
}

/** Grade de SubtelaCard — menu inicial de uma página dividida em subtelas. */
export function SubtelasMenu({ opcoes }: { opcoes: { id: string; icon: string; label: string; descricao: string; onClick: () => void }[] }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {opcoes.map((o) => <SubtelaCard key={o.id} icon={o.icon} label={o.label} descricao={o.descricao} onClick={o.onClick} />)}
    </div>
  );
}

/**
 * Tabela única de cobranças, com botão "Ver detalhes" explícito por linha
 * e, quando `onCancelar` é fornecido, um botão "Cancelar" independente
 * (na própria linha, não dentro do detalhe — cancelar é uma ação sobre a
 * cobrança, não parte de "ler os detalhes dela").
 */
export function CobrancasTable({ rows, onOpen, onCancelar }: {
  rows: CobrancaResumo[];
  onOpen: (r: CobrancaResumo) => void;
  onCancelar?: (r: CobrancaResumo, motivo?: string) => Promise<void>;
}) {
  const [cancelandoId, setCancelandoId] = useState<string | null>(null);
  const [erro, setErro] = useState<string | null>(null);
  const cancelavel = (r: CobrancaResumo) => !["success", "pago", "cancelado", "cancelled", "failed", "falhado"].includes(r.status.toLowerCase());

  return (
    <div className="space-y-2">
      {erro && <p className="text-sm text-error-500">{erro}</p>}
      <div className="overflow-x-auto">
        <Table className="w-full text-left">
          <TableHeader className="border-b border-gray-100 dark:border-white/[0.05]">
            <TableRow>
              {["Tipo", "Descrição", "Estudante", "Valor", "Método", "Estado", "Atualizado em", ""].map((h) => (
                <TableCell key={h || "acoes"} isHeader className="px-3 py-2 text-xs font-medium uppercase text-gray-500 dark:text-gray-400">{h}</TableCell>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody className="divide-y divide-gray-100 dark:divide-white/[0.05]">
            {rows.map((r) => (
              <TableRow key={r.id}>
                <TableCell className="px-3 py-2 text-sm text-gray-700 dark:text-gray-300">{origemLabel[r.origem] ?? r.origem}</TableCell>
                <TableCell className="px-3 py-2 text-sm text-gray-700 dark:text-gray-300">{r.descricao || "—"}</TableCell>
                <TableCell className="px-3 py-2 text-sm text-gray-700 dark:text-gray-300">{r.codigo_estudante || "—"}</TableCell>
                <TableCell className="px-3 py-2 text-sm text-gray-700 dark:text-gray-300">{money(r.valor)}</TableCell>
                <TableCell className="px-3 py-2 text-sm text-gray-700 dark:text-gray-300">{r.metodo_pagamento || "—"}</TableCell>
                <TableCell className="px-3 py-2"><StatusBadge status={r.status} /></TableCell>
                <TableCell className="px-3 py-2 text-sm text-gray-700 dark:text-gray-300">{dt(r.atualizado_em)}</TableCell>
                <TableCell className="px-3 py-2">
                  <div className="flex flex-wrap gap-2">
                    <Button size="sm" variant="outline" onClick={() => onOpen(r)}>Ver detalhes</Button>
                    {onCancelar && cancelavel(r) && (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={cancelandoId === r.id}
                        onClick={async () => {
                          setErro(null);
                          setCancelandoId(r.id);
                          try {
                            await onCancelar(r, window.prompt("Motivo do cancelamento (opcional)") || undefined);
                          } catch (e) {
                            setErro(formatApiError(e, "Não foi possível cancelar a cobrança."));
                          } finally {
                            setCancelandoId(null);
                          }
                        }}
                      >
                        Cancelar
                      </Button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

/**
 * Subtela de detalhes de uma cobrança (não é mais modal/pop-up).
 *
 * - Usa os dados já carregados na linha da tabela (CobrancaResumo) em vez
 *   de buscar a cobrança de novo no servidor — evita uma requisição
 *   redundante a cada "ver detalhes" (a listagem já trouxe tudo que a
 *   cobrança tem).
 * - Quando a cobrança está vinculada a um estudante (codigo_estudante) e
 *   mostrarDadosEstudante=true, busca e exibe também os dados desse
 *   estudante. GET /consultar-estudante/:codigo só é permitido para
 *   academia/admin — por isso EstudantePagamentosPainel usa
 *   mostrarDadosEstudante={false} (o estudante já sabe quem é).
 * - Não tem ação de cancelar: cancelar é uma ação sobre a cobrança na
 *   listagem (CobrancasTable, botão "Cancelar" na própria linha), não faz
 *   parte de "ler os detalhes" dela.
 */
export function SubtelaDetalheCobranca({ cobranca, onVoltar, mostrarDadosEstudante = false }: {
  cobranca: CobrancaResumo;
  onVoltar: () => void;
  mostrarDadosEstudante?: boolean;
}) {
  const [estudante, setEstudante] = useState<EstudanteDetalhado | null>(null);
  const [erroEstudante, setErroEstudante] = useState<string | null>(null);
  const [carregandoEstudante, setCarregandoEstudante] = useState(false);

  const codigoEstudante = cobranca.codigo_estudante;

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect -- reseta o estado de estudante ao trocar de cobrança, antes de buscar os novos dados.
    setEstudante(null);
    setErroEstudante(null);
    if (!mostrarDadosEstudante || !codigoEstudante) return;
    setCarregandoEstudante(true);
    consultasService.estudante(codigoEstudante)
      .then((r) => setEstudante(r?.estudante ?? null))
      .catch((e) => setErroEstudante(formatApiError(e, "Não foi possível carregar os dados do estudante.")))
      .finally(() => setCarregandoEstudante(false));
  }, [cobranca.id, mostrarDadosEstudante, codigoEstudante]);

  return (
    <SubtelaPanel title="Detalhe da cobrança" icon="mdi:receipt-text-outline" onVoltar={onVoltar}>
      <div className="space-y-3 text-sm text-gray-700 dark:text-gray-300">
        <p><b>Tipo:</b> {origemLabel[cobranca.origem] ?? cobranca.origem}</p>
        <p><b>Descrição:</b> {cobranca.descricao || "—"}</p>
        <p><b>Valor:</b> {money(cobranca.valor)} {cobranca.moeda ? `(${cobranca.moeda})` : ""}</p>
        <p><b>Método de pagamento:</b> {cobranca.metodo_pagamento || "—"}</p>
        <p><b>Estado:</b> <StatusBadge status={cobranca.status} /></p>
        <p><b>Referência AppyPay:</b> {cobranca.provider_charge_id || "—"}</p>
        <p><b>Transação:</b> {cobranca.merchant_transaction_id}</p>
        <p><b>Atualizado em:</b> {dt(cobranca.atualizado_em)}</p>
        {cobranca.codigo_solicitacao && <p><b>Solicitação de matrícula:</b> {cobranca.codigo_solicitacao}</p>}

        {codigoEstudante && (
          <div className="mt-4 rounded-lg border border-gray-100 p-3 dark:border-white/[0.05]">
            <p className="mb-2 font-semibold text-gray-800 dark:text-white/90">Estudante vinculado</p>
            {!mostrarDadosEstudante ? (
              <p><b>Código:</b> {codigoEstudante}</p>
            ) : carregandoEstudante ? (
              <p className="text-gray-500 dark:text-gray-400">Carregando dados do estudante...</p>
            ) : erroEstudante ? (
              <p className="text-red-600 dark:text-red-400">{erroEstudante}</p>
            ) : estudante ? (
              <div className="space-y-1">
                <p><b>Nome:</b> {estudante.nome}</p>
                <p><b>Código:</b> {estudante.codigo_estudante}</p>
                {estudante.telefone && <p><b>Telefone:</b> {estudante.telefone}</p>}
                {estudante.email && <p><b>Email:</b> {estudante.email}</p>}
                {estudante.status && <p><b>Status:</b> {estudante.status}</p>}
              </div>
            ) : (
              <p><b>Código:</b> {codigoEstudante}</p>
            )}
          </div>
        )}
      </div>
    </SubtelaPanel>
  );
}
```

---

## Seção 5 — `spuripainel/src/components/paineis/FinanceiroConfiguracoesPainel.tsx` (substituir arquivo inteiro)

**Objetivo, mapeado ponto a ponto ao feedback:**

- **Terminologia** — o rótulo "Fundamental" (que nunca deveria ter existido — a tarefa 49 original já usava "Nª Classe" para o *ano/classe*, mas não corrigiu o rótulo do *nível* em si) agora vem de `NIVEL_LABEL` (`financeiroShared.tsx`): "Ensino Primário e Iº Ciclo", "Médio", "Superior" — igual ao resto do painel.
- **Nível inferido da academia** — antes havia sempre um select fixo com as 3 opções (fundamental/médio/superior). Agora `niveisDaAcademia(user.academia)` decide: se a academia só oferece um nível (o caso mais comum), **nenhum select aparece** — o nível já vem aplicado, mostrado como texto informativo. Só quando a academia é `nivel_escolar="misto"" (oferece fundamental E médio) é que aparece um SearchableSelect, e mesmo assim só com essas duas opções (nunca "Superior" numa escola, nunca "Fundamental"/"Médio" numa faculdade).
- **Cursos e anos continuam inferidos dos dados reais da academia** (isso já era assim na tarefa 49: `academia.anos_academicos` para fundamental, `academiaService.listarCursos` filtrado por nível para médio/superior) — mantido.
- **Ano letivo formatado** — `formatAnoLetivo` em vez do `.replace("_","/")` ad-hoc que já existia; agora é a mesma função usada em todo o módulo (Seção 4).
- **"Ações excecionais" deixou de existir como seção única** — "Definir início de cobrança fora do padrão" e "Anular ou reativar obrigações" agora são **duas subtelas separadas** (item do feedback: "secção para anular ou reativar obrigações deve ser separada").
- **A página inteira foi dividida em subtelas** em vez de seções empilhadas: o que era uma página com 4 seções (propina, matrícula, histórico, ações excecionais) na mesma tela agora é um **menu** (`SubtelasMenu`) com 5 cartões — Propina/mensalidade, Taxa de matrícula, Histórico de versões, Início de cobrança fora do padrão, Anular ou reativar obrigações — cada um abrindo sua própria subtela (`SubtelaPanel`, com botão Voltar).
- **Nenhum checkbox nativo** — "Métodos de pagamento aceites" já usava o componente `Checkbox` desde a tarefa 49; mantido sem alteração (era o exemplo de referência citado no feedback).
- Visão de admin (FPP) continua mostrando só o aviso "indisponível no momento" — sem alteração nessa parte.

SUBSTITUIR O ARQUIVO INTEIRO por:

```tsx
"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { academiaService, financeiroService, useApi } from "@/lib/api";
import { formatApiError } from "@/lib/api/client";
import { useUserType } from "@/hooks/useRoutePermission";
import UnauthorizedAccess from "@/components/guards/UnauthorizedAccess";
import Alert from "@/components/ui/alert/Alert";
import Button from "@/components/ui/button/Button";
import Icon from "@/components/ui/Icon";
import Input from "@/components/form/input/InputField";
import Label from "@/components/form/Label";
import SearchableSelect from "@/components/form/SearchableSelect";
import Checkbox from "@/components/form/input/Checkbox";
import AnularReativarObrigacoesForm from "@/components/paineis/AnularReativarObrigacoesForm";
import { LoadingState, NIVEL_LABEL, SubtelaPanel, SubtelasMenu, formatAnoLetivo, money, niveisDaAcademia } from "@/components/paineis/financeiroShared";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "@/components/ui/table";
import type { Curso, FinanceiroMetodoPagamento, FinanceiroNivel, MatriculaConfiguracaoInput, MensalidadeConfiguracaoInput } from "@/types/api";

const METODOS: FinanceiroMetodoPagamento[] = ["GPO", "REF", "GPO_QR"];
const MES_FIM_OPCOES = [
  { value: "6", label: "Junho" },
  { value: "7", label: "Julho" },
];
/** Nomes reais dos meses (pt-AO) — corrige o bug de exibir "Mês 1", "Mês 2"... */
const MES_NOME_OPCOES = Array.from({ length: 12 }, (_, i) => ({
  value: String(i + 1),
  label: new Intl.DateTimeFormat("pt-AO", { month: "long" }).format(new Date(2026, i, 1)),
}));

function date(value: string) { const d = new Date(value); return Number.isNaN(d.getTime()) ? value : new Intl.DateTimeFormat("pt-AO", { dateStyle: "short", timeStyle: "short" }).format(d); }

/** "6_ano_fundamental" → "6ª Classe"; "2_ano_medio" → "2.º Ano (Médio)". Mesmo padrão usado nas telas de matrícula/turmas. */
function labelAnoAcademico(codigo: string): string {
  const m = /^(\d+)_ano_(fundamental|medio|superior)$/.exec(codigo);
  if (!m) return codigo;
  const [, numero, nivel] = m;
  if (nivel === "fundamental") return `${numero}ª Classe`;
  if (nivel === "medio") return `${numero}.º Ano (Médio)`;
  return `${numero}.º Ano (Superior)`;
}

function InfoBox() {
  return (
    <div className="rounded-xl border border-brand-200 bg-brand-50 p-4 dark:border-brand-900 dark:bg-brand-900/20">
      <div className="flex items-start gap-3">
        <Icon icon="mdi:information-outline" width={20} className="mt-0.5 shrink-0 text-brand-600 dark:text-brand-300" />
        <div>
          <p className="text-sm font-semibold text-brand-700 dark:text-brand-200">Regras financeiras importantes</p>
          <ul className="mt-2 list-disc space-y-1 pl-5 text-sm text-brand-700/90 dark:text-brand-300">
            <li>Cada configuração enviada cria uma <b>nova versão vigente a partir de agora</b> — não edita nem apaga versões passadas. Meses e matrículas já vencidos continuam usando o valor que estava vigente na época em que venceram.</li>
            <li>A configuração é específica por <b>nível de ensino</b> e, dentro dele, por <b>ano/classe</b> (fundamental) ou por <b>curso e ano</b> (médio/superior) — por isso pode (e normalmente deve) haver várias configurações vigentes ao mesmo tempo, uma por combinação.</li>
            <li>Na Matrícula: se <b>nenhuma</b> configuração existir para a combinação nível/ano/curso de uma solicitação, a matrícula daquele candidato é <b>gratuita</b> e a academia aprova direto, sem cobrança.</li>
            <li>Pagamentos só podem ser feitos pelos métodos habilitados aqui: <b>GPO</b> (Multicaixa Express via número de telefone), <b>REF</b> (referência para pagar em qualquer Multicaixa/ATM/homebanking) e <b>GPO_QR</b> (QR Code, exibido para o pagador escanear no momento em que ele escolhe pagar).</li>
            <li>É <b>obrigatório configurar as credenciais AppyPay antes</b> — sem isso, nenhuma cobrança pode ser criada mesmo com o valor já configurado aqui. <Link href="/financas/credenciais" className="font-medium underline">Configurar credenciais</Link>.</li>
          </ul>
        </div>
      </div>
    </div>
  );
}

type NivelFormState = {
  nivel: FinanceiroNivel;
  ano_academico: string;
  curso_id: string;
  valor: string;
  metodos_pagamento: FinanceiroMetodoPagamento[];
};

type FormFieldErrors = Partial<Record<"ano_academico" | "curso_id" | "valor", string>>;

type Tela = "menu" | "mensalidade" | "matricula" | "historico" | "inicio-cobranca" | "anular-reativar";

/**
 * Painel de configurações financeiras, dividido em subtelas (não seções
 * empilhadas na mesma página): cada configuração — propina, matrícula,
 * histórico, início de cobrança fora do padrão, anular/reativar
 * obrigações — é a sua própria subtela, aberta a partir de um menu.
 *
 * Visão de admin (FPP): configuração de propina/matrícula é uma
 * responsabilidade exclusiva de cada academia — não existe hoje nenhuma
 * configuração financeira que pertença ao administrador. Por isso o admin
 * não vê o menu de subtelas: só o aviso "indisponível no momento".
 */
export default function FinanceiroConfiguracoesPainel() {
  const { user, isAdmin, isAcademia, loading } = useUserType();
  const isFpp = isAdmin && user?.admin?.role === "fpp";
  const codigoAcademia = user?.academia?.codigo_academia ?? "";
  const anosAcademicosAcademia = useMemo(() => user?.academia?.anos_academicos ?? [], [user?.academia?.anos_academicos]);
  /** Níveis que a academia realmente oferece — nunca uma lista fixa fundamental/médio/superior. */
  const niveisDisponiveis = useMemo(() => niveisDaAcademia(user?.academia), [user?.academia]);

  const [tela, setTela] = useState<Tela>("menu");
  const [alert, setAlert] = useState<{ variant: "success" | "error" | "warning" | "info"; message: string } | null>(null);
  const [mensalidadeForm, setMensalidadeForm] = useState<NivelFormState>({ nivel: niveisDisponiveis[0] ?? "fundamental", ano_academico: "", curso_id: "", valor: "", metodos_pagamento: ["GPO"] });
  const [mensalidadeMesFim, setMensalidadeMesFim] = useState("6");
  const [mensalidadeErrors, setMensalidadeErrors] = useState<FormFieldErrors>({});
  const [matriculaForm, setMatriculaForm] = useState<NivelFormState>({ nivel: niveisDisponiveis[0] ?? "fundamental", ano_academico: "", curso_id: "", valor: "", metodos_pagamento: ["GPO"] });
  const [matriculaErrors, setMatriculaErrors] = useState<FormFieldErrors>({});
  const [cursos, setCursos] = useState<Curso[]>([]);

  const mensalidadesApi = useApi(financeiroService.listarConfiguracoesMensalidade);
  const matriculasApi = useApi(financeiroService.listarConfiguracoesMatricula);
  const salvarMensalidade = useApi(financeiroService.configurarMensalidade);
  const salvarMatricula = useApi(financeiroService.configurarMatricula);
  const atualizarMensalidade = useApi(financeiroService.atualizarConfiguracaoMensalidade);
  const atualizarMatricula = useApi(financeiroService.atualizarConfiguracaoMatricula);

  useEffect(() => {
    if (niveisDisponiveis.length === 0) return;
    setMensalidadeForm((prev) => (niveisDisponiveis.includes(prev.nivel) ? prev : { ...prev, nivel: niveisDisponiveis[0], curso_id: "", ano_academico: "" }));
    setMatriculaForm((prev) => (niveisDisponiveis.includes(prev.nivel) ? prev : { ...prev, nivel: niveisDisponiveis[0], curso_id: "", ano_academico: "" }));
  }, [niveisDisponiveis]);

  const reload = async () => {
    if (!codigoAcademia) return;
    await Promise.all([
      mensalidadesApi.execute({ codigo_academia: codigoAcademia }),
      matriculasApi.execute({ codigo_academia: codigoAcademia }),
    ]);
  };

  useEffect(() => {
    if (!loading && isAcademia && codigoAcademia) void reload().catch((err) => setAlert({ variant: "error", message: formatApiError(err, "Não foi possível carregar configurações.") }));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loading, isAcademia, codigoAcademia]);

  useEffect(() => {
    if (!isAcademia || !codigoAcademia) { setCursos([]); return; }
    academiaService.listarCursos({ codigo_academia: codigoAcademia })
      .then((r) => setCursos((r.cursos ?? []).filter((c) => c.status === "ativo")))
      .catch(() => setCursos([]));
  }, [isAcademia, codigoAcademia]);

  if (loading) return <LoadingState label="Carregando configurações..." />;
  if (!isAcademia && !isFpp) return <UnauthorizedAccess requiredTypes={["Admin FPP", "Academia"]} message="O módulo financeiro é exclusivo de administradores FPP e academias." />;

  if (isFpp) {
    return (
      <section className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/[0.05] dark:bg-white/[0.03]">
        <div className="flex items-start gap-3">
          <Icon icon="mdi:cog-outline" width={24} className="text-gray-800 dark:text-white/90" />
          <div>
            <h2 className="text-xl font-semibold text-gray-800 dark:text-white/90">Configurações financeiras</h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Propina, matrícula e as demais configurações desta página pertencem a cada academia, não ao administrador —
              indisponível no momento. Ainda não existe nenhuma configuração financeira própria do Spuri.
            </p>
          </div>
        </div>
      </section>
    );
  }

  const toggleMetodo = (kind: "mensalidade" | "matricula", metodo: FinanceiroMetodoPagamento) => {
    const setter = kind === "mensalidade" ? setMensalidadeForm : setMatriculaForm;
    setter((prev) => ({ ...prev, metodos_pagamento: prev.metodos_pagamento.includes(metodo) ? prev.metodos_pagamento.filter((m) => m !== metodo) : [...prev.metodos_pagamento, metodo] }));
  };

  const cursosDoNivel = (nivel: FinanceiroNivel) => cursos.filter((c) => c.type === nivel);
  const anosDoFormulario = (form: NivelFormState): string[] => {
    if (form.nivel === "fundamental") return anosAcademicosAcademia.filter((a) => a.endsWith("_ano_fundamental"));
    const curso = cursos.find((c) => c.id === form.curso_id);
    return curso?.anos_academicos ?? [];
  };

  const validarValorEAno = (form: NivelFormState): FormFieldErrors => {
    const errors: FormFieldErrors = {};
    const valorNumero = Number(form.valor);
    if (!form.valor.trim() || !(valorNumero > 0)) errors.valor = "Informe um valor maior que zero.";
    if (form.nivel === "fundamental") {
      if (!form.ano_academico) errors.ano_academico = "Selecione o ano/classe.";
    } else {
      if (!form.curso_id) errors.curso_id = "Selecione o curso.";
      if (!form.ano_academico) errors.ano_academico = "Selecione o ano do curso.";
    }
    return errors;
  };

  const matches = (c: { nivel: string; curso_id?: string; ano_academico?: string }, form: NivelFormState) =>
    c.nivel === form.nivel && (form.nivel === "fundamental" ? c.ano_academico === form.ano_academico : c.curso_id === form.curso_id && c.ano_academico === form.ano_academico);

  const submitMensalidade = async () => {
    const errors = validarValorEAno(mensalidadeForm);
    setMensalidadeErrors(errors);
    if (Object.keys(errors).length > 0) return;
    try {
      if (!codigoAcademia) throw new Error("Academia não identificada.");
      const p: MensalidadeConfiguracaoInput = {
        codigo_academia: codigoAcademia,
        nivel: mensalidadeForm.nivel,
        ano_academico: mensalidadeForm.ano_academico,
        curso_id: mensalidadeForm.nivel === "fundamental" ? undefined : mensalidadeForm.curso_id,
        valor: Number(mensalidadeForm.valor),
        mes_fim_cobranca: Number(mensalidadeMesFim) as 6 | 7,
        metodos_pagamento: mensalidadeForm.metodos_pagamento,
      };
      const exists = (mensalidadesApi.data?.configuracoes ?? []).some((c) => matches(c, mensalidadeForm));
      await (exists ? atualizarMensalidade.execute(p) : salvarMensalidade.execute(p));
      setAlert({ variant: "success", message: "Configuração de mensalidade versionada com sucesso." });
      await reload();
    } catch (err) {
      setAlert({ variant: "error", message: formatApiError(err, "Não foi possível salvar mensalidade.") });
    }
  };

  const submitMatricula = async () => {
    const errors = validarValorEAno(matriculaForm);
    setMatriculaErrors(errors);
    if (Object.keys(errors).length > 0) return;
    try {
      if (!codigoAcademia) throw new Error("Academia não identificada.");
      const p: MatriculaConfiguracaoInput = {
        codigo_academia: codigoAcademia,
        nivel: matriculaForm.nivel,
        ano_academico: matriculaForm.ano_academico,
        curso_id: matriculaForm.nivel === "fundamental" ? undefined : matriculaForm.curso_id,
        valor: Number(matriculaForm.valor),
        metodos_pagamento: matriculaForm.metodos_pagamento,
      };
      const exists = (matriculasApi.data?.configuracoes ?? []).some((c) => matches(c, matriculaForm));
      await (exists ? atualizarMatricula.execute(p) : salvarMatricula.execute(p));
      setAlert({ variant: "success", message: "Configuração de matrícula versionada com sucesso." });
      await reload();
    } catch (err) {
      setAlert({ variant: "error", message: formatApiError(err, "Não foi possível salvar matrícula.") });
    }
  };

  const updateNivel = (kind: "mensalidade" | "matricula", nivel: FinanceiroNivel) => {
    const setter = kind === "mensalidade" ? setMensalidadeForm : setMatriculaForm;
    const setErrors = kind === "mensalidade" ? setMensalidadeErrors : setMatriculaErrors;
    setter((prev) => ({ ...prev, nivel, curso_id: "", ano_academico: "" }));
    setErrors({});
  };

  const renderMetodos = (kind: "mensalidade" | "matricula", selected: FinanceiroMetodoPagamento[]) => (
    <div className="flex flex-wrap gap-4">
      {METODOS.map((m) => (
        <Checkbox key={m} id={`${kind}-metodo-${m}`} label={m} checked={selected.includes(m)} onChange={() => toggleMetodo(kind, m)} />
      ))}
    </div>
  );

  /** Nível só aparece como escolha quando a academia oferece mais de um (ex.: nivel_escolar="misto"). Com um único nível, ele é aplicado direto, sem select. */
  const renderNivelFields = (kind: "mensalidade" | "matricula", form: NivelFormState, errors: FormFieldErrors, setForm: (updater: (prev: NivelFormState) => NivelFormState) => void) => (
    <>
      {niveisDisponiveis.length > 1 ? (
        <>
          <Label>Nível</Label>
          <SearchableSelect
            value={form.nivel}
            options={niveisDisponiveis.map((n) => ({ value: n, label: NIVEL_LABEL[n] }))}
            onChange={(v) => updateNivel(kind, (v || niveisDisponiveis[0]) as FinanceiroNivel)}
            isSearchable={false}
            isClearable={false}
            inputId={`${kind}-nivel`}
            name={`${kind}-nivel`}
          />
        </>
      ) : (
        <p className="text-sm text-gray-500 dark:text-gray-400">Nível: <span className="font-medium text-gray-800 dark:text-white/90">{NIVEL_LABEL[form.nivel]}</span></p>
      )}
      {form.nivel !== "fundamental" && (
        <>
          <Label>Curso</Label>
          <SearchableSelect
            value={form.curso_id}
            options={cursosDoNivel(form.nivel).map((c) => ({ value: c.id, label: c.nome }))}
            onChange={(v) => setForm((prev) => ({ ...prev, curso_id: v, ano_academico: "" }))}
            placeholder={cursosDoNivel(form.nivel).length ? "Selecione um curso" : "Nenhum curso cadastrado para este nível"}
            isClearable
            inputId={`${kind}-curso`}
            name={`${kind}-curso`}
            error={errors.curso_id}
          />
        </>
      )}
      <Label>{form.nivel === "fundamental" ? "Ano / classe" : "Ano do curso"}</Label>
      <SearchableSelect
        value={form.ano_academico}
        options={anosDoFormulario(form).map((a) => ({ value: a, label: labelAnoAcademico(a) }))}
        onChange={(v) => setForm((prev) => ({ ...prev, ano_academico: v }))}
        placeholder={anosDoFormulario(form).length ? "Selecione o ano" : "Selecione um curso primeiro"}
        isDisabled={form.nivel !== "fundamental" && !form.curso_id}
        isClearable
        inputId={`${kind}-ano-academico`}
        name={`${kind}-ano-academico`}
        error={errors.ano_academico}
      />
      <Label>Valor (Kz)</Label>
      <Input
        type="number"
        min="0.01"
        step={0.01}
        value={form.valor}
        onChange={(e) => setForm((prev) => ({ ...prev, valor: e.target.value }))}
        error={!!errors.valor}
        hint={errors.valor}
      />
    </>
  );

  if (tela === "menu") {
    return (
      <div className="space-y-6">
        {alert && <Alert variant={alert.variant} title="Finanças" message={alert.message} />}
        <InfoBox />
        <SubtelasMenu
          opcoes={[
            { id: "mensalidade", icon: "mdi:calendar-month-outline", label: "Propina / mensalidade", descricao: "Definir o valor e os métodos aceites por ano/curso.", onClick: () => setTela("mensalidade") },
            { id: "matricula", icon: "mdi:school-outline", label: "Taxa de matrícula", descricao: "Definir o valor e os métodos aceites por ano/curso.", onClick: () => setTela("matricula") },
            { id: "historico", icon: "mdi:history", label: "Histórico de versões", descricao: "Ver todas as configurações já salvas, com a data de vigência.", onClick: () => setTela("historico") },
            { id: "inicio-cobranca", icon: "mdi:calendar-start", label: "Início de cobrança fora do padrão", descricao: "Ajustar a partir de qual mês a propina passa a valer num ano letivo específico.", onClick: () => setTela("inicio-cobranca") },
            { id: "anular-reativar", icon: "mdi:receipt-text-remove-outline", label: "Anular ou reativar obrigações", descricao: "Anular ou reativar mensalidades pontuais de um estudante específico.", onClick: () => setTela("anular-reativar") },
          ]}
        />
      </div>
    );
  }

  if (tela === "mensalidade") {
    return (
      <SubtelaPanel title="Propina / mensalidade" icon="mdi:calendar-month-outline" onVoltar={() => setTela("menu")}>
        {alert && <Alert variant={alert.variant} title="Finanças" message={alert.message} />}
        <div className="grid gap-4">
          {renderNivelFields("mensalidade", mensalidadeForm, mensalidadeErrors, setMensalidadeForm)}
          <Label>Mês de encerramento da cobrança</Label>
          <SearchableSelect
            value={mensalidadeMesFim}
            options={MES_FIM_OPCOES}
            onChange={(v) => setMensalidadeMesFim(v || "6")}
            isSearchable={false}
            isClearable={false}
            inputId="mensalidade-mes-fim"
            name="mensalidade-mes-fim"
          />
          <Label>Métodos de pagamento aceites</Label>
          {renderMetodos("mensalidade", mensalidadeForm.metodos_pagamento)}
          <Button onClick={submitMensalidade} disabled={salvarMensalidade.loading || atualizarMensalidade.loading} startIcon={<Icon icon="mdi:content-save-outline" width={16} />}>
            Salvar nova versão
          </Button>
        </div>
      </SubtelaPanel>
    );
  }

  if (tela === "matricula") {
    return (
      <SubtelaPanel title="Taxa de matrícula" icon="mdi:school-outline" onVoltar={() => setTela("menu")}>
        {alert && <Alert variant={alert.variant} title="Finanças" message={alert.message} />}
        <div className="grid gap-4">
          {renderNivelFields("matricula", matriculaForm, matriculaErrors, setMatriculaForm)}
          <Label>Métodos de pagamento aceites</Label>
          {renderMetodos("matricula", matriculaForm.metodos_pagamento)}
          <Button onClick={submitMatricula} disabled={salvarMatricula.loading || atualizarMatricula.loading} startIcon={<Icon icon="mdi:content-save-outline" width={16} />}>
            Salvar nova versão
          </Button>
        </div>
      </SubtelaPanel>
    );
  }

  if (tela === "historico") {
    const linhas = [
      ...(mensalidadesApi.data?.configuracoes ?? []).map((c) => ({ tipo: "Propina", fim: String(c.mes_fim_cobranca), ...c })),
      ...(matriculasApi.data?.configuracoes ?? []).map((c) => ({ tipo: "Matrícula", fim: "—", ...c })),
    ];
    return (
      <SubtelaPanel title="Histórico de versões" icon="mdi:history" onVoltar={() => setTela("menu")}>
        <div className="overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>{["Tipo", "Nível", "Ano/Curso", "Valor", "Fim", "Métodos", "Vigente em"].map((h) => <TableCell key={h} isHeader className="px-3 py-2 text-xs uppercase text-gray-500 dark:text-gray-400">{h}</TableCell>)}</TableRow>
            </TableHeader>
            <TableBody>
              {linhas.map((c, i) => (
                <TableRow key={i}>
                  <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{c.tipo}</TableCell>
                  <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{NIVEL_LABEL[c.nivel]}</TableCell>
                  <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{c.ano_academico ? labelAnoAcademico(c.ano_academico) : (cursos.find((cu) => cu.id === c.curso_id)?.nome ?? c.curso_id ?? "—")}</TableCell>
                  <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{money(c.valor)}</TableCell>
                  <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{c.fim === "6" ? "Junho" : c.fim === "7" ? "Julho" : c.fim}</TableCell>
                  <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{c.metodos_pagamento.join(", ")}</TableCell>
                  <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{date(c.vigente_em)}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </SubtelaPanel>
    );
  }

  if (tela === "inicio-cobranca") {
    return (
      <SubtelaPanel title="Início de cobrança fora do padrão" icon="mdi:calendar-start" onVoltar={() => setTela("menu")}>
        <p className="text-sm text-gray-500 dark:text-gray-400">
          Use apenas se o ano letivo começou fora do padrão (ex.: turma que iniciou em março em vez de fevereiro) — isso ajusta a partir de qual mês a cobrança de propina passa a valer para esse ano letivo.
        </p>
        <div className="mt-4">
          <DefinirInicioCobrancaForm codigoAcademia={codigoAcademia} />
        </div>
      </SubtelaPanel>
    );
  }

  // tela === "anular-reativar"
  return (
    <SubtelaPanel title="Anular ou reativar obrigações" icon="mdi:receipt-text-remove-outline" onVoltar={() => setTela("menu")}>
      <p className="text-sm text-gray-500 dark:text-gray-400">Anule ou reative mensalidades pontuais de um estudante específico (ex.: bolsa concedida, erro de lançamento).</p>
      <div className="mt-4">
        <AnularReativarObrigacoesForm codigoAcademia={codigoAcademia} onSuccess={reload} />
      </div>
    </SubtelaPanel>
  );
}

/**
 * Formulário de "definir início de cobrança fora do padrão". Extraído do
 * corpo do painel para poder buscar o ano letivo real da academia (em vez
 * de texto livre) sem misturar essa busca com o resto do estado da página.
 */
function DefinirInicioCobrancaForm({ codigoAcademia }: { codigoAcademia: string }) {
  const [anosLetivos, setAnosLetivos] = useState<string[]>([]);
  const [anoLetivo, setAnoLetivo] = useState("");
  const [mesInicio, setMesInicio] = useState("2");
  const [alert, setAlert] = useState<{ variant: "success" | "error"; message: string } | null>(null);
  const definirInicio = useApi(financeiroService.definirInicioCobranca);

  useEffect(() => {
    if (!codigoAcademia) return;
    Promise.all([
      academiaService.getAnoLetivo({ codigo_academia: codigoAcademia }),
      academiaService.listarAnosLetivosLista({ codigo_academia: codigoAcademia }),
    ]).then(([atual, lista]) => {
      const anos = Array.from(new Set([atual?.ano_letivo, ...((lista?.anos_letivos_lista ?? []).map((a) => a.ano_letivo))].filter((a): a is string => !!a)));
      setAnosLetivos(anos);
      setAnoLetivo((prev) => prev || atual?.ano_letivo || anos[0] || "");
    }).catch(() => setAnosLetivos([]));
  }, [codigoAcademia]);

  const submit = async () => {
    setAlert(null);
    if (!anoLetivo) { setAlert({ variant: "error", message: "Selecione o ano letivo." }); return; }
    try {
      await definirInicio.execute({ codigo_academia: codigoAcademia, ano_letivo: anoLetivo, mes_inicio: Number(mesInicio) });
      setAlert({ variant: "success", message: "Início de cobrança definido com sucesso." });
    } catch (err) {
      setAlert({ variant: "error", message: formatApiError(err, "Não foi possível definir o início de cobrança.") });
    }
  };

  return (
    <div className="space-y-3">
      {alert && <Alert variant={alert.variant} title="Início de cobrança" message={alert.message} />}
      <div className="grid gap-4 sm:grid-cols-[1fr_1fr_auto]">
        <div>
          <Label>Ano letivo</Label>
          <SearchableSelect
            value={anoLetivo}
            options={anosLetivos.map((a) => ({ value: a, label: formatAnoLetivo(a) }))}
            onChange={(v) => setAnoLetivo(v)}
            placeholder={anosLetivos.length ? "Selecione o ano letivo" : "Nenhum ano letivo definido para esta academia"}
            isSearchable={false}
            inputId="inicio-cobranca-ano-letivo"
            name="inicio-cobranca-ano-letivo"
          />
        </div>
        <div>
          <Label>Mês início</Label>
          <SearchableSelect
            value={mesInicio}
            options={MES_NOME_OPCOES}
            onChange={(v) => setMesInicio(v || "2")}
            isSearchable={false}
            isClearable={false}
            inputId="inicio-cobranca-mes"
            name="inicio-cobranca-mes"
          />
        </div>
        <div className="self-end">
          <Button onClick={submit} disabled={!anoLetivo || definirInicio.loading} startIcon={<Icon icon="mdi:calendar-start" width={16} />}>
            Definir início de cobrança
          </Button>
        </div>
      </div>
    </div>
  );
}
```

---

## Seção 6 — `spuripainel/src/components/paineis/AnularReativarObrigacoesForm.tsx` (substituir arquivo inteiro)

**Objetivo:** troca só o `.replace("_","/")` ad-hoc por `formatAnoLetivo` (Seção 4), para consistência com o resto do módulo. Este componente já não usa nem select nativo nem checkbox nativo (usa `MultiSelect`, um componente próprio, não o `<input type=checkbox>` do navegador). Continua sendo renderizado como sua própria subtela a partir do menu de `FinanceiroConfiguracoesPainel.tsx` (Seção 5), não mais dentro de uma seção compartilhada com "Definir início de cobrança".

SUBSTITUIR O ARQUIVO INTEIRO por:

```tsx
"use client";
import { useEffect, useState } from "react";
import { academiaService, consultasService, financeiroService, useApi } from "@/lib/api";
import { formatApiError } from "@/lib/api/client";
import SearchableSelect from "@/components/form/SearchableSelect";
import MultiSelect from "@/components/form/MultiSelect";
import Input from "@/components/form/input/InputField";
import Label from "@/components/form/Label";
import Button from "@/components/ui/button/Button";
import Alert from "@/components/ui/alert/Alert";
import Icon from "@/components/ui/Icon";
import { formatAnoLetivo } from "@/components/paineis/financeiroShared";

/** Nomes reais dos meses — corrige o bug de exibir "Mês 1", "Mês 2"... */
const MESES = Array.from({ length: 12 }, (_, i) => ({
  value: String(i + 1),
  text: new Intl.DateTimeFormat("pt-AO", { month: "long" }).format(new Date(2026, i, 1)),
  selected: false,
}));

export default function AnularReativarObrigacoesForm({ codigoAcademia, onSuccess }: { codigoAcademia: string; onSuccess?: () => void }) {
  const [estudantes, setEstudantes] = useState<{ value: string; label: string }[]>([]);
  const [codigoEstudante, setCodigoEstudante] = useState("");
  const [anosLetivos, setAnosLetivos] = useState<string[]>([]);
  const [anoLetivo, setAnoLetivo] = useState("");
  const [meses, setMeses] = useState<string[]>([]);
  const [motivo, setMotivo] = useState("");
  const [alert, setAlert] = useState<{ variant: "success" | "error"; message: string } | null>(null);
  const anular = useApi(financeiroService.anularObrigacoes);
  const reativar = useApi(financeiroService.reativarObrigacoes);

  useEffect(() => {
    if (!codigoAcademia) return;
    consultasService.listarEstudantes({ codigo_academia: codigoAcademia, limit: 300, offset: 0 })
      .then((r) => setEstudantes((r.estudantes ?? []).map((e: any) => ({ value: e.codigo_estudante, label: `${e.nome ?? e.codigo_estudante} (${e.codigo_estudante})` }))))
      .catch(() => setEstudantes([]));
  }, [codigoAcademia]);

  useEffect(() => {
    if (!codigoAcademia) return;
    Promise.all([
      academiaService.getAnoLetivo({ codigo_academia: codigoAcademia }),
      academiaService.listarAnosLetivosLista({ codigo_academia: codigoAcademia }),
    ]).then(([atual, lista]) => {
      const anos = Array.from(new Set([atual?.ano_letivo, ...((lista?.anos_letivos_lista ?? []).map((a) => a.ano_letivo))].filter((a): a is string => !!a)));
      setAnosLetivos(anos);
      setAnoLetivo((prev) => prev || atual?.ano_letivo || anos[0] || "");
    }).catch(() => setAnosLetivos([]));
  }, [codigoAcademia]);

  const executar = async (acao: "anular" | "reativar") => {
    if (!codigoEstudante || !anoLetivo || meses.length === 0) { setAlert({ variant: "error", message: "Selecione o estudante, o ano letivo e ao menos um mês." }); return; }
    if (acao === "anular" && !motivo.trim()) { setAlert({ variant: "error", message: "Informe o motivo para anular obrigações." }); return; }
    try {
      const payload = { codigo_estudante: codigoEstudante, codigo_academia: codigoAcademia, ano_letivo: anoLetivo, meses: meses.map(Number), motivo: motivo.trim() || undefined };
      await (acao === "anular" ? anular.execute(payload) : reativar.execute(payload));
      setAlert({ variant: "success", message: acao === "anular" ? "Obrigações anuladas." : "Obrigações reativadas." });
      onSuccess?.();
    } catch (err) {
      setAlert({ variant: "error", message: formatApiError(err, "Não foi possível concluir a ação.") });
    }
  };

  return <div className="space-y-4 rounded-xl bg-gray-50 p-4 dark:bg-white/[0.03]">
    {alert && <Alert variant={alert.variant} title="Obrigações de mensalidade" message={alert.message} />}
    <div><Label>Estudante</Label><SearchableSelect value={codigoEstudante} options={estudantes} onChange={setCodigoEstudante} placeholder="Buscar estudante..." isClearable /></div>
    <div className="grid gap-4 sm:grid-cols-2">
      <div>
        <Label>Ano letivo</Label>
        <SearchableSelect
          value={anoLetivo}
          options={anosLetivos.map((a) => ({ value: a, label: formatAnoLetivo(a) }))}
          onChange={setAnoLetivo}
          placeholder={anosLetivos.length ? "Selecione o ano letivo" : "Nenhum ano letivo definido para esta academia"}
          isSearchable={false}
          inputId="anular-reativar-ano-letivo"
          name="anular-reativar-ano-letivo"
        />
      </div>
      <MultiSelect label="Meses" options={MESES} defaultSelected={meses} onChange={setMeses} />
    </div>
    <div><Label>Motivo (obrigatório para anular)</Label><Input value={motivo} onChange={(e) => setMotivo(e.target.value)} placeholder="Ex.: bolsa concedida, erro de lançamento..." /></div>
    <div className="flex gap-3"><Button size="sm" variant="outline" disabled={anular.loading} onClick={() => executar("anular")} startIcon={<Icon icon="mdi:close-circle-outline" width={16}/>}>Anular selecionados</Button><Button size="sm" disabled={reativar.loading} onClick={() => executar("reativar")} startIcon={<Icon icon="mdi:reload" width={16}/>}>Reativar selecionados</Button></div>
  </div>;
}
```

---

## Seção 7 — `spuripainel/src/components/paineis/FinanceiroPagamentosPainel.tsx` (substituir arquivo inteiro)

**Objetivo:** "ver detalhes" agora abre `SubtelaDetalheCobranca` (substitui a tabela na tela, com botão Voltar) em vez do antigo `<Modal>`. "Cancelar" deixou de estar dentro do detalhe — agora é um botão "Cancelar" na própria linha da tabela (via o novo parâmetro `onCancelar` de `CobrancasTable`, Seção 4), visível só quando a cobrança daquela linha pode ser cancelada. O resto (paginação de 30, filtros de tipo/estado, aviso de indisponibilidade para admin) não muda.

SUBSTITUIR O ARQUIVO INTEIRO por:

```tsx
"use client";
import { useCallback, useEffect, useMemo, useState } from "react";
import { financeiroService, useApi } from "@/lib/api";
import { formatApiError } from "@/lib/api/client";
import { useUserType } from "@/hooks/useRoutePermission";
import UnauthorizedAccess from "@/components/guards/UnauthorizedAccess";
import Alert from "@/components/ui/alert/Alert";
import Icon from "@/components/ui/Icon";
import SearchableSelect from "@/components/form/SearchableSelect";
import {
  CobrancasTable,
  EmptyState,
  LoadingState,
  PaginacaoSetas,
  SubtelaDetalheCobranca,
} from "@/components/paineis/financeiroShared";
import type { CobrancaResumo, FinanceiroOrigemCobranca } from "@/types/api";

const PAGE_SIZE = 30;

const TIPO_OPCOES: { value: "" | FinanceiroOrigemCobranca; label: string }[] = [
  { value: "", label: "Todos os tipos" },
  { value: "mensalidade", label: "Mensalidade" },
  { value: "matricula", label: "Matrícula" },
  { value: "avulsa", label: "Outros" },
];

const ESTADO_OPCOES = [
  { value: "", label: "Todos os estados" },
  { value: "Success", label: "Pago" },
  { value: "Pending", label: "Pendente" },
  { value: "Failed", label: "Falhado" },
  { value: "Cancelled", label: "Cancelado" },
];

/**
 * Painel de pagamentos para academia e admin (FPP).
 *
 * - Academia: uma única tabela de cobranças, paginada de verdade (30 por
 *   página, requisição sempre com limit/offset da página atual — mesmo
 *   padrão de /estudantes), com filtro por tipo e por estado. Cada
 *   cobrança tem seu botão "Ver detalhes", que abre uma SUBTELA (não um
 *   pop-up) no lugar da tabela; "Cancelar" é uma ação independente, na
 *   própria linha da tabela.
 * - Admin (FPP): ainda não existe tipo de cobrança específico para o
 *   Spuri, então a tela mostra apenas um aviso "indisponível no momento".
 */
export default function FinanceiroPagamentosPainel() {
  const { user, isAdmin, isAcademia, loading } = useUserType();
  const isFpp = isAdmin && user?.admin?.role === "fpp";

  const [codigoAcademia, setCodigoAcademia] = useState(user?.academia?.codigo_academia ?? "");
  const [tipo, setTipo] = useState<"" | FinanceiroOrigemCobranca>("");
  const [estado, setEstado] = useState("");
  const [pagina, setPagina] = useState(1);
  const [alert, setAlert] = useState<string | null>(null);
  const [selecionada, setSelecionada] = useState<CobrancaResumo | null>(null);

  const list = useApi(financeiroService.listarCobrancas);
  const cancelApi = useApi(financeiroService.cancelarCobranca);

  useEffect(() => {
    if (user?.academia?.codigo_academia) setCodigoAcademia(user.academia.codigo_academia);
  }, [user?.academia?.codigo_academia]);

  const parametros = useMemo(
    () => ({
      contexto_tipo: "academia" as const,
      codigo_academia: codigoAcademia || undefined,
      limit: PAGE_SIZE,
      offset: (pagina - 1) * PAGE_SIZE,
      tipo: tipo ? [tipo] : undefined,
      estado: estado ? [estado] : undefined,
    }),
    [codigoAcademia, tipo, estado, pagina]
  );

  const carregar = useCallback(() => {
    if (!codigoAcademia) return Promise.resolve();
    return list.execute(parametros).catch((e) => setAlert(formatApiError(e, "Não foi possível carregar as cobranças.")));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [codigoAcademia, parametros]);

  useEffect(() => {
    if (!loading && isAcademia) void carregar();
  }, [loading, isAcademia, carregar]);

  if (loading) return <LoadingState label="Carregando pagamentos..." />;

  if (!isAcademia && !isFpp) {
    return (
      <UnauthorizedAccess
        requiredTypes={["Admin FPP", "Academia"]}
        message="O módulo financeiro é exclusivo de administradores com papel FPP e de academias."
      />
    );
  }

  if (isFpp) {
    return (
      <section className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/[0.05] dark:bg-white/[0.03]">
        <div className="flex items-start gap-3">
          <Icon icon="mdi:credit-card-multiple-outline" width={24} className="text-gray-800 dark:text-white/90" />
          <div>
            <h2 className="text-xl font-semibold text-gray-800 dark:text-white/90">Pagamentos</h2>
            <p className="mt-1 text-sm text-gray-500 dark:text-gray-400">
              Ainda não existe um tipo de cobrança específico para o Spuri — indisponível no momento.
            </p>
          </div>
        </div>
      </section>
    );
  }

  if (selecionada) {
    return (
      <SubtelaDetalheCobranca
        cobranca={selecionada}
        onVoltar={() => setSelecionada(null)}
        mostrarDadosEstudante
      />
    );
  }

  const totalGeral = list.data?.total_geral ?? 0;
  const totalPaginas = Math.max(1, Math.ceil(totalGeral / PAGE_SIZE));
  const cobrancas = list.data?.cobrancas ?? [];

  return (
    <div className="space-y-6">
      {alert && <Alert variant="error" title="Finanças" message={alert} />}

      <section className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/[0.05] dark:bg-white/[0.03]">
        <div className="flex items-start gap-3">
          <Icon icon="mdi:credit-card-multiple-outline" width={24} className="text-gray-800 dark:text-white/90" />
          <div>
            <h2 className="text-xl font-semibold text-gray-800 dark:text-white/90">Pagamentos</h2>
            <p className="text-sm text-gray-500 dark:text-gray-400">Cobranças AppyPay da sua academia, em todos os estados.</p>
          </div>
        </div>

        <div className="mt-4 grid gap-3 sm:grid-cols-2">
          <SearchableSelect
            value={tipo}
            options={TIPO_OPCOES}
            onChange={(v) => { setTipo(v); setPagina(1); }}
            placeholder="Tipo de cobrança"
            isSearchable={false}
            isClearable={false}
            inputId="pagamentos-tipo"
            name="pagamentos-tipo"
          />
          <SearchableSelect
            value={estado}
            options={ESTADO_OPCOES}
            onChange={(v) => { setEstado(v); setPagina(1); }}
            placeholder="Estado do pagamento"
            isSearchable={false}
            isClearable={false}
            inputId="pagamentos-estado"
            name="pagamentos-estado"
          />
        </div>

        <div className="mt-4">
          {list.loading ? (
            <LoadingState label="Carregando pagamentos..." />
          ) : cobrancas.length > 0 ? (
            <CobrancasTable
              rows={cobrancas}
              onOpen={setSelecionada}
              onCancelar={async (cobranca, motivo) => {
                await cancelApi.execute(cobranca.id, motivo);
                await carregar();
              }}
            />
          ) : (
            <EmptyState title="Nenhuma cobrança encontrada." description="Ajuste os filtros ou aguarde novas cobranças serem criadas." />
          )}
        </div>

        <div className="mt-4">
          <PaginacaoSetas
            paginaAtual={pagina}
            totalPaginas={totalPaginas}
            total={totalGeral}
            porPagina={PAGE_SIZE}
            onChange={setPagina}
          />
        </div>
      </section>
    </div>
  );
}
```

---

## Seção 8 — `spuripainel/src/components/paineis/EstudantePagamentosPainel.tsx` (substituir arquivo inteiro)

**Objetivo:**
- "Ver detalhes" no histórico agora abre `SubtelaDetalheCobranca` em vez do antigo `<Modal>` (sem botão de cancelar, estudante nunca teve essa ação mesmo).
- **"Pagar mensalidades" deixou de ser um `<Modal>`** — agora é sua própria subtela (`SubtelaPanel`), com Voltar retornando para "Meus pagamentos".
- **Corrige o checkbox nativo que restava**: a seleção de meses a pagar usava `<input type="checkbox">` puro dentro do modal de pagamento — trocado pelo componente `Checkbox` (o mesmo já usado em "Métodos de pagamento aceites"), citado como referência de UI no feedback.
- **Ano letivo formatado** (`formatAnoLetivo`) nas duas tabelas que mostravam o valor cru ("Meus pagamentos" e a lista de meses pendentes na subtela de pagamento).
- A seção "Meus pagamentos" continua sendo a mesma funcionalidade, sem alteração de regra de negócio — só a forma de abrir o fluxo de pagamento (subtela em vez de pop-up) e o checkbox mudaram.

SUBSTITUIR O ARQUIVO INTEIRO por:

```tsx
"use client";
import { useEffect, useMemo, useState } from "react";
import { financeiroService, tokenStorage, useApi } from "@/lib/api";
import { formatApiError } from "@/lib/api/client";
import { useUserCookie } from "@/hooks/useUserCookie";
import Alert from "@/components/ui/alert/Alert";
import Button from "@/components/ui/button/Button";
import Input from "@/components/form/input/InputField";
import Select from "@/components/form/Select";
import SearchableSelect from "@/components/form/SearchableSelect";
import Checkbox from "@/components/form/input/Checkbox";
import { Table, TableBody, TableCell, TableHeader, TableRow } from "@/components/ui/table";
import {
  CobrancasTable,
  EmptyState,
  LoadingState,
  PaginacaoSetas,
  Qr,
  StatusBadge,
  SubtelaDetalheCobranca,
  SubtelaPanel,
  formatAnoLetivo,
  money,
} from "@/components/paineis/financeiroShared";
import type { CobrancaResumo, FinanceiroMetodoPagamento, FinanceiroOrigemCobranca, MensalidadeMesView } from "@/types/api";

const PAGE_SIZE = 30;
const mesNome = (m: number) => new Intl.DateTimeFormat("pt-AO", { month: "long" }).format(new Date(2026, m - 1, 1));
function getCodigo(user: any) { return user?.estudante?.codigo_estudante || user?.estudante?.codigo || user?.codigo; }

const TIPO_OPCOES: { value: "" | FinanceiroOrigemCobranca; label: string }[] = [
  { value: "", label: "Todos os tipos" },
  { value: "mensalidade", label: "Mensalidade" },
  { value: "matricula", label: "Matrícula" },
  { value: "avulsa", label: "Outros" },
];

const ESTADO_HISTORICO_OPCOES = [
  { value: "", label: "Todos os estados" },
  { value: "Success", label: "Pago" },
  { value: "Pending", label: "Pendente" },
  { value: "Failed", label: "Falhado" },
  { value: "Cancelled", label: "Cancelado" },
];

type Tela = { nome: "lista" } | { nome: "pagar"; academia: string } | { nome: "detalhe"; cobranca: CobrancaResumo };

export default function EstudantePagamentosPainel() {
  const { user, loading } = useUserCookie();
  const restricted = tokenStorage.isRestrictedFinance();
  const codigo = getCodigo(user);

  const [tela, setTela] = useState<Tela>({ nome: "lista" });

  // ── Mensalidades pendentes + pagamento (feature independente, não alterada na regra de negócio) ──
  const mensalidades = useApi(financeiroService.consultarMensalidadesEstudante);
  const pagar = useApi(financeiroService.iniciarPagamentoMensalidades);
  const [estadoMensalidades, setEstadoMensalidades] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [metodo, setMetodo] = useState<FinanceiroMetodoPagamento>("GPO");
  const [telefone, setTelefone] = useState("");
  const [result, setResult] = useState<any>(null);

  // ── Histórico completo de cobranças (tipo + estado + ver detalhes) ──
  const historico = useApi(financeiroService.consultarCobrancasEstudante);
  const [tipoHistorico, setTipoHistorico] = useState<"" | FinanceiroOrigemCobranca>("");
  const [estadoHistorico, setEstadoHistorico] = useState("");
  const [paginaHistorico, setPaginaHistorico] = useState(1);

  const [alert, setAlert] = useState<string | null>(null);

  useEffect(() => {
    if (!loading && codigo) {
      void mensalidades.execute(codigo).catch((e) => setAlert(formatApiError(e, "Não foi possível carregar mensalidades.")));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loading, codigo]);

  useEffect(() => {
    if (!loading && codigo && !restricted) {
      void historico
        .execute(codigo, {
          limit: PAGE_SIZE,
          offset: (paginaHistorico - 1) * PAGE_SIZE,
          tipo: tipoHistorico ? [tipoHistorico] : undefined,
          estado: estadoHistorico ? [estadoHistorico] : undefined,
        })
        .catch((e) => setAlert(formatApiError(e, "Não foi possível carregar o histórico de cobranças.")));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [loading, codigo, restricted, paginaHistorico, tipoHistorico, estadoHistorico]);

  const filtered = (mensalidades.data?.mensalidades ?? []).filter((m) => !estadoMensalidades || m.estado === estadoMensalidades);
  const byAcademia = useMemo(
    () => filtered.reduce<Record<string, MensalidadeMesView[]>>((acc, m) => { (acc[m.codigo_academia] ??= []).push(m); return acc; }, {}),
    [filtered]
  );

  const abrirPagamento = (academia: string, meses: MensalidadeMesView[]) => {
    const pend = meses.filter((m) => m.estado === "pendente").sort((a, b) => a.ano_letivo.localeCompare(b.ano_letivo) || a.mes - b.mes);
    setSelected(pend[0] ? [`${pend[0].ano_letivo}:${pend[0].mes}`] : []);
    setMetodo((mensalidades.data?.metodos_pagamento_por_academia[academia]?.[0] ?? "GPO") as FinanceiroMetodoPagamento);
    setTelefone("");
    setResult(null);
    setTela({ nome: "pagar", academia });
  };

  const confirmarPagamento = async (academia: string) => {
    try {
      const meses = selected.map((x) => { const [ano_letivo, mes] = x.split(":"); return { ano_letivo, mes: Number(mes) }; });
      const r = await pagar.execute({ codigo_academia: academia, meses, metodo_pagamento: metodo, telefone: metodo === "GPO" ? telefone : undefined });
      setResult(r);
      await mensalidades.execute(codigo);
    } catch (e) {
      setAlert(formatApiError(e, "Não foi possível iniciar o pagamento."));
    }
  };

  if (loading) return <LoadingState label="Carregando..." />;
  if (!codigo) return <Alert variant="error" title="Pagamentos" message="Não foi possível identificar o estudante logado." />;

  if (tela.nome === "detalhe") {
    return <SubtelaDetalheCobranca cobranca={tela.cobranca} onVoltar={() => setTela({ nome: "lista" })} mostrarDadosEstudante={false} />;
  }

  if (tela.nome === "pagar") {
    const academia = tela.academia;
    const pendentes = (mensalidades.data?.mensalidades ?? [])
      .filter((m) => m.codigo_academia === academia && m.estado === "pendente")
      .sort((a, b) => a.ano_letivo.localeCompare(b.ano_letivo) || a.mes - b.mes);
    return (
      <SubtelaPanel title={`Pagar mensalidades — Academia ${academia}`} icon="mdi:cash-multiple" onVoltar={() => setTela({ nome: "lista" })}>
        {alert && <Alert variant="error" title="Pagamentos" message={alert} />}
        {result ? (
          <div className="space-y-3">
            <p className="text-sm text-gray-700 dark:text-gray-300">Status: {result.cobranca.status}</p>
            {metodo === "GPO" && <p className="text-sm text-gray-700 dark:text-gray-300">Você receberá uma notificação no telefone informado para confirmar o pagamento.</p>}
            {metodo === "REF" && <pre className="rounded bg-gray-50 p-3 text-xs dark:bg-gray-800">{JSON.stringify(result.cobranca.response ?? {}, null, 2)}</pre>}
            {metodo === "GPO_QR" && <Qr value={result.cobranca.qrCodeArr} />}
            <Button size="sm" onClick={() => void mensalidades.execute(codigo)}>Verificar status</Button>
          </div>
        ) : (
          <div className="space-y-4">
            <div className="space-y-2">
              {pendentes.map((m, i) => {
                const key = `${m.ano_letivo}:${m.mes}`;
                return (
                  <Checkbox
                    key={key}
                    id={`mes-${key}`}
                    checked={selected.includes(key)}
                    disabled={i === 0}
                    onChange={(checked) => setSelected((s) => (checked ? [...s, key] : s.filter((x) => x !== key)))}
                    label={`${formatAnoLetivo(m.ano_letivo)} · ${mesNome(m.mes)} · ${money(m.valor)}${i === 0 ? " (mais antigo, obrigatório)" : ""}`}
                  />
                );
              })}
            </div>
            <Select
              key={metodo}
              defaultValue={metodo}
              options={(mensalidades.data?.metodos_pagamento_por_academia[academia] ?? ["GPO"]).map((m) => ({ value: m, label: m }))}
              onChange={(v) => setMetodo(v as FinanceiroMetodoPagamento)}
            />
            {metodo === "GPO" && <Input placeholder="Telefone" value={telefone} onChange={(e) => setTelefone(e.target.value)} />}
            <Button disabled={!selected.length || (metodo === "GPO" && !telefone)} onClick={() => confirmarPagamento(academia)}>Confirmar pagamento</Button>
          </div>
        )}
      </SubtelaPanel>
    );
  }

  const totalHistorico = historico.data?.total_geral ?? 0;
  const totalPaginasHistorico = Math.max(1, Math.ceil(totalHistorico / PAGE_SIZE));

  return (
    <div className="space-y-6">
      {restricted && <Alert variant="warning" title="Acesso financeiro restrito" message="O seu vínculo com a academia foi encerrado. Você pode consultar e regularizar pendências financeiras aqui." />}
      {alert && <Alert variant="error" title="Pagamentos" message={alert} />}

      <section className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/[0.05] dark:bg-white/[0.03]">
        <h1 className="text-xl font-semibold text-gray-800 dark:text-white/90">Meus pagamentos</h1>
        <div className="mt-4">
          <Select
            key={estadoMensalidades}
            defaultValue={estadoMensalidades}
            options={[{ value: "", label: "Todos estados" }, { value: "pendente", label: "Pendentes" }, { value: "pago", label: "Pagos" }, { value: "anulado", label: "Anulados" }]}
            onChange={(v) => setEstadoMensalidades(v)}
          />
        </div>
        {Object.entries(byAcademia).map(([academia, rows]) => (
          <div key={academia} className="mt-5 rounded-xl border p-4 dark:border-white/[0.05]">
            <div className="flex items-center justify-between">
              <h2 className="font-semibold text-gray-800 dark:text-white/90">Academia {academia}</h2>
              {rows.some((m) => m.estado === "pendente") && <Button size="sm" onClick={() => abrirPagamento(academia, rows)}>Pagar mensalidades</Button>}
            </div>
            <div className="mt-3 overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>{["Ano letivo", "Mês", "Valor", "Estado"].map((h) => <TableCell key={h} isHeader className="px-3 py-2 text-xs uppercase text-gray-500 dark:text-gray-400">{h}</TableCell>)}</TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((m, i) => (
                    <TableRow key={i}>
                      <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{formatAnoLetivo(m.ano_letivo)}</TableCell>
                      <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{mesNome(m.mes)}</TableCell>
                      <TableCell className="px-3 py-2 text-gray-700 dark:text-gray-300">{money(m.valor)}</TableCell>
                      <TableCell className="px-3 py-2"><StatusBadge status={m.estado} /></TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </div>
        ))}
      </section>

      <section className="rounded-2xl border border-gray-200 bg-white p-5 dark:border-white/[0.05] dark:bg-white/[0.03]">
        <h2 className="font-semibold text-gray-800 dark:text-white/90">Histórico completo de cobranças</h2>
        {restricted ? (
          <p className="mt-2 text-sm text-gray-500 dark:text-gray-400">Histórico completo indisponível nesta sessão restrita; apenas mensalidades e pagamento estão liberados.</p>
        ) : (
          <>
            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <SearchableSelect
                value={tipoHistorico}
                options={TIPO_OPCOES}
                onChange={(v) => { setTipoHistorico(v); setPaginaHistorico(1); }}
                placeholder="Tipo de cobrança"
                isSearchable={false}
                isClearable={false}
                inputId="historico-tipo"
                name="historico-tipo"
              />
              <SearchableSelect
                value={estadoHistorico}
                options={ESTADO_HISTORICO_OPCOES}
                onChange={(v) => { setEstadoHistorico(v); setPaginaHistorico(1); }}
                placeholder="Estado do pagamento"
                isSearchable={false}
                isClearable={false}
                inputId="historico-estado"
                name="historico-estado"
              />
            </div>
            <div className="mt-4">
              {historico.loading ? (
                <LoadingState label="Carregando histórico..." />
              ) : (historico.data?.cobrancas?.length ?? 0) > 0 ? (
                <CobrancasTable rows={historico.data?.cobrancas ?? []} onOpen={(c) => setTela({ nome: "detalhe", cobranca: c })} />
              ) : (
                <EmptyState title="Sem histórico." description="Nenhuma cobrança foi encontrada para os filtros selecionados." />
              )}
            </div>
            <div className="mt-4">
              <PaginacaoSetas paginaAtual={paginaHistorico} totalPaginas={totalPaginasHistorico} total={totalHistorico} porPagina={PAGE_SIZE} onChange={setPaginaHistorico} />
            </div>
          </>
        )}
      </section>
    </div>
  );
}
```

---

## Fora de escopo (não fazer)

- `FinanceiroCredenciaisPainel.tsx` não muda nesta rodada (ver Resumo executivo) — não editar.
- Não tocar em nenhuma migração, nem em `internal/finance/mensalidade.go`/`internal/finance/matricula.go` além do que já foi validado (nenhuma mudança de negócio nova nesta rodada, só a extensão do filtro de tipo).
- Não alterar a seção "Meus pagamentos" além do que está descrito na Seção 8 (checkbox e formatação de ano letivo) — a regra de negócio de pagamento de mensalidades continua igual.
- Não tocar em `MatriculaPublicPage.tsx` — o import de `Qr`/`money` de `financeiroShared.tsx` continua válido, essas duas funções não mudaram de assinatura.
- Não mexer em nenhum outro arquivo do repositório além dos 8 listados no resumo executivo.

## Critérios de aceitação

1. `cd spuri-backend && go build ./...`, `go vet ./...` e `gofmt -l internal/finance/appypay.go internal/handlers/financeiro_handlers.go internal/finance/cobrancas_estudante_integration_test.go` terminam sem erro/saída.
2. `cd spuripainel && npx tsc --noEmit` termina sem nenhum erro.
3. `cd spuripainel && npx eslint src/components/paineis/financeiroShared.tsx src/components/paineis/FinanceiroConfiguracoesPainel.tsx src/components/paineis/AnularReativarObrigacoesForm.tsx src/components/paineis/FinanceiroPagamentosPainel.tsx src/components/paineis/EstudantePagamentosPainel.tsx` termina sem erro/aviso.
4. Estudante consegue filtrar o histórico de cobranças por tipo (mensalidade/matrícula/outros) em `/pagamentos` e o resultado realmente muda.
5. Uma academia que só oferece um nível de ensino não vê nenhum select de "Nível" nas telas de propina/matrícula — o nível já vem aplicado.
6. Em nenhuma tela do módulo financeiro (`/financas/*` e `/pagamentos`) existe mais `<input type="checkbox">` nativo nem `<Modal>`/pop-up — tudo é subtela (título + botão Voltar) ou o componente `Checkbox`.
7. "Cancelar" nunca aparece dentro da tela "Detalhe da cobrança" — só como botão independente na linha da tabela.
8. `/financas/configuracoes` abre num menu de 5 opções; cada uma é sua própria subtela, sem mais de uma configuração visível ao mesmo tempo na tela.

## Procedimento de conclusão

```bash
cd spuri-backend
go build ./...
go vet ./...
gofmt -l internal/finance/appypay.go internal/handlers/financeiro_handlers.go internal/finance/cobrancas_estudante_integration_test.go
go test ./internal/finance/... -run TestIntegrationListCobrancasEstudante -v

cd ../spuripainel
npx tsc --noEmit
npx eslint src/components/paineis/financeiroShared.tsx src/components/paineis/FinanceiroConfiguracoesPainel.tsx src/components/paineis/AnularReativarObrigacoesForm.tsx src/components/paineis/FinanceiroPagamentosPainel.tsx src/components/paineis/EstudantePagamentosPainel.tsx
```

O `go test` só funciona com PostgreSQL local acessível (`DATABASE_URL`, `RUN_POSTGRES_INTEGRATION=1`, `FINANCE_ENCRYPTION_KEY` definidos) — se o ambiente do Codex não tiver isso, pule esse comando e reporte os demais; Claude já validou com PostgreSQL 16 real neste documento (dois testes passando, incluindo os 4 casos novos do filtro por tipo).

Depois de mover esta tarefa para `docs/Tarefas feitas/`, atualize o frontmatter `status` para `concluída`.
