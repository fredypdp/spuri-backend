---
modificado: 06-03-2026
criado: 04-03-2026 17:05
---
> Executar em etapas, **uma por chamada**, na mesma ordem da auditoria.  
> Cada etapa corrige apenas os arquivos auditados naquela fase.

---

## CONTEXTO FIXO (repetir em todas as etapas)

O projeto é **spuri-backend**, uma API em Go que usa **Event Sourcing + CQRS**.  
Toda mutação de estado deve obrigatoriamente passar pelo ledger (`spuri_ledger`) antes de atualizar qualquer projeção.  
O sistema deve ser: **Seguro · Auditável · Confiável · Sólido**.

**Referência obrigatória antes de corrigir:**

- Leia o arquivo de auditoria da etapa atual (`Prompt de Auditoria — spuri-backend.md`) para conhecer todos os erros a corrigir
- Leia os arquivos originais completos antes de reescrever qualquer um — nunca assuma o conteúdo de memória
- Mantenha total consistência com os arquivos de etapas anteriores já corrigidos

---

## PRÉ-PROCESSAMENTO OBRIGATÓRIO (executar antes de escrever qualquer código)

> **Esta etapa deve ser concluída integralmente antes de tocar em qualquer arquivo.**

Antes de iniciar as correções, faça o seguinte mapeamento a partir do arquivo de auditoria da etapa:

1. **Agrupe todos os bugs por arquivo** — percorra a auditoria do início ao fim e monte uma lista no formato:
   ```
   arquivo.go
     → [BUG-01] descrição resumida
     → [BUG-03] descrição resumida
     → [BUG-07] descrição resumida
   outro_arquivo.go
     → [BUG-02] descrição resumida
     → [BUG-05] descrição resumida
   ```

2. **Ordene os arquivos** pela sequência de dependência (ex: `aggregate.go` antes de `estudante.go`), não pela ordem em que os bugs aparecem na auditoria.

3. **Só então comece a escrever código** — ao abrir um arquivo para corrigir, aplique **todos os bugs mapeados para aquele arquivo de uma vez**, sem exceção. Nunca feche um arquivo com bugs pendentes para resolver "depois".

> **Motivação:** bugs espalhados ao longo da auditoria que afetam o mesmo arquivo causam reescritas repetidas e risco de regressão. O agrupamento elimina isso garantindo que cada arquivo seja reescrito uma única vez, já com todas as correções incorporadas.

---

## REGRAS DE CORREÇÃO (válidas em todas as etapas)

1. **Retorne cada arquivo corrigido como um artefact separado** — nunca cole código no corpo do chat
2. **Retorne o arquivo completo** — sem omissões, sem `// ... resto igual`, sem truncamento
3. **Fique atento ao contexto atual do código** — não introduza padrões novos que quebrem o restante do projeto
4. **Valide atomicamente** antes de entregar:
    - Sem `redeclared` — nenhum símbolo declarado duas vezes no mesmo escopo
    - Sem `undefined` — todo símbolo usado existe e está importado
    - Sem import não utilizado
    - Sem função/método referenciado que não existe em outro arquivo da etapa
5. **Não misture lógicas diferentes num mesmo arquivo** — respeite a separação já existente no projeto (ex: `estudante_notas.go` só contém lógica de notas)
6. **Se um arquivo precisar ser removido**, indique claramente em texto (fora do artefact): `🗑️ REMOVER: caminho/do/arquivo.go — motivo`
7. **Se uma migration nova for necessária**, crie-a como artefact separado com nome sequencial correto
8. **Não altere arquivos que não têm erros** — menos mudança = menos risco de regressão

---

## VALIDAÇÃO FINAL (executar ao fim de cada etapa)

Antes de encerrar a etapa, percorra **um por um** todos os arquivos que você modificou ou criou e responda internamente:

- [ ] O arquivo compila isoladamente? (imports corretos, sem símbolo undefined)
- [ ] Todos os métodos referenciados existem no projeto (nesta etapa ou em etapas anteriores)?
- [ ] Nenhum erro do `auditoria-etapaN.md` ficou sem correção?
- [ ] Nenhuma correção introduziu um novo bug (ex: campo nil não tratado, versão não incrementada)?
- [ ] O comportamento de rebuild da projeção continua correto após as mudanças?

Se alguma resposta for **não**, corrija antes de entregar.

No chat, ao final da etapa, poste apenas:

```
✅ Etapa N corrigida.
Arquivos modificados: X
Arquivos novos: Y
Arquivos a remover: Z (listados acima)
Pendências para próxima etapa: [se houver dependências que a próxima etapa precisa saber]
```

---

## ETAPA 1 — Correção de `/domain`

**Entrada:** `auditoria-etapa1-domain.md`  
**Arquivos em escopo:** todos em `internal/domain/`

Ordem de correção sugerida:

1. `aggregate.go` — base que todos dependem
2. `admin.go`
3. `academia.go` + `academia_categorias_nota.go`
4. `estudante.go`
5. `estudante_falta.go`, `estudante_notas.go`, `estudante_avaliacao.go`, `estudante_aprovacao.go`
6. Demais arquivos em `/domain`

---

## ETAPA 2 — Correção de `/db`

**Entrada:** `auditoria-etapa2-db.md` + arquivos de `/domain` já corrigidos na Etapa 1  
**Arquivos em escopo:** todos em `internal/db/` e `migrations/`

Ordem de correção sugerida:

1. `safe_queries.go` — whitelist de eventos (deve refletir todos os eventos do domain corrigido)
2. `event_store.go` / `repository.go` — atomicidade de Save, ordem de Load
3. `client.go` — se houver erros
4. Migrations novas (se necessário para suportar correções de schema)

---

## ETAPA 3 — Correção de `/projections`

**Entrada:** `auditoria-etapa3-projections.md` + domain e db já corrigidos  
**Arquivos em escopo:** todos em `internal/projections/`

Ordem de correção sugerida:

1. Projection com mais erros primeiro (conforme auditoria)
2. Garantir que cada projection trata exatamente os eventos que o aggregate emite
3. Verificar `Rebuild()` em todos

---

## ETAPA 4 — Correção de `/handlers`

**Entrada:** `auditoria-etapa4-handlers.md` + todas as etapas anteriores já corrigidas  
**Arquivos em escopo:** `internal/handlers/`, `internal/middleware/`, arquivo de rotas

Ordem de correção sugerida:

1. `middleware/` — auth e extração de contexto (base de todos os handlers)
2. Handlers por domínio: admin → academia → estudante → demais
3. Arquivo de rotas — validar que todas as rotas têm middleware correto aplicado

---

## INSTRUÇÃO FINAL (repetir em todas as etapas)

> Leia **todos** os arquivos da etapa antes de escrever qualquer correção.  
> Execute o **pré-processamento** — agrupe todos os bugs por arquivo antes de tocar em qualquer código.  
> Corrija **todos** os erros listados na auditoria da etapa — sem deixar nenhum para depois.  
> Entregue **somente artefacts** — zero código no corpo do chat.  
> Execute a **validação final** antes de encerrar.
