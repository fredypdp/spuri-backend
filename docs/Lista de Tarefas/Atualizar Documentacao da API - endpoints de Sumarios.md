---
criado: 29-08-2026
status: pronto_para_execucao
tipo: documentacao
patch: tarefa_doc_api_sumarios.patch
depende_de: "Tarefa de Sumários (já aplicada) — este documento só atualiza a Documentação da API.md para refletir o que já está no ar"
---

# Atualizar `Documentação da API.md` com os endpoints de Sumários

## 0. O que aconteceu

A funcionalidade de Sumários (`Sumario` aggregate, `POST/GET/PUT/DELETE /academia/sumario(s)`, vínculo opcional em faltas via `sumario_id`/`sumario_titulo`, endpoint `PUT /academia/faltas-aluno/:id/desvincular-sumario`) foi aplicada e mesclada em `main`, mas a atualização da `Documentação da API.md` — prática padrão neste repositório para mudanças de contrato de API (ver Tarefas 48, 51 e a atualização de endpoints da Tarefa 74) — ficou pendente. Confirmei isso lendo o arquivo atual: não há nenhuma menção a "sumario" nele.

Este documento cobre só isso: puro markdown, sem migration, sem Go, sem risco de regressão de código. Escrevi o patch lendo o código-fonte já mesclado (`sumario_handlers.go`, `sumarios_projection.go`, os trechos alterados de `faltas_handlers.go`, e as rotas em `cmd/server/main.go`) para garantir que a documentação reflete exatamente o comportamento real, não a especificação original da tarefa. Testei o patch: `git apply --check` limpo e `git apply` produzindo exatamente o conteúdo esperado, num clone independente e recente de `main`.

## 1. Prompt recomendado

> Aplique `tarefa_doc_api_sumarios.patch` na raiz do repositório (`git apply tarefa_doc_api_sumarios.patch`) e confirme visualmente que o markdown renderiza corretamente (sem blocos de código quebrados, sem headers duplicados). Não precisa rodar `go build`/testes — este patch toca só `Documentação da API.md`. Depois, mova este documento para `docs/Tarefas feitas/` seguindo a convenção normal.

## 2. O que o patch muda em `Documentação da API.md`

| Seção | Mudança |
|---|---|
| Frontmatter | versão `2.4.0 → 2.5.0` |
| Índice | nova entrada `22. Sumários` |
| `### 2.11 Falta` | `FaltaDTO` ganha `sumario_id?` e `sumario_titulo?` |
| `### 2.13 Registro de Falta (consulta global)` | `FaltaRegistroDTO` ganha os mesmos dois campos |
| **`### 2.14 Sumário`** (nova) | interface `SumarioDTO` completa — preenche uma lacuna de numeração que já existia no arquivo (havia dois `2.13` e o próximo era `2.15`, sem nenhum `2.14`) |
| `## 14. Faltas` — parágrafo introdutório | uma frase sobre o vínculo opcional com sumário |
| `POST /academia/faltas-aluno` | `sumario_id` (opcional) no request; regra de compatibilidade (mesma matéria/período/ano_academico) |
| `PATCH /academia/faltas-aluno/:id` | `sumario_id` (opcional) no request; as 3 semânticas (omitido preserva, valor troca, `null` é rejeitado) explicadas |
| **`PUT /academia/faltas-aluno/:id/desvincular-sumario`** (nova) | endpoint completo, inserido logo após o `PATCH`; documentado como idempotente |
| `GET /faltas` | `sumario_id`/`sumario_titulo` no exemplo de resposta |
| `GET /faltas-estudante/:codigo` | os mesmos dois campos no exemplo (e uma nota indicando que o exemplo já era parcial antes deste patch — não é algo que este patch tenta corrigir por completo, só evita que o exemplo pareça sugerir que os campos novos não existem) |
| **`## 22. Sumários`** (nova seção, ao final do documento) | `GET /academia/sumarios`, `GET /academia/sumario/:id`, `POST /academia/sumario`, `PUT /academia/sumario/:id/dados`, `DELETE /academia/sumario/:id` |

Todas as entradas novas seguem exatamente o formato já usado no resto do arquivo (`**Proteção**:`, `**Request:**`, `**Response 200:**`, `**Erros:**` em bullet/tabela), conferido contra as seções de Cursos e Matérias como referência antes de escrever.

**Por que a nova seção foi ao final (22) em vez de logo depois de Faltas (que viraria 15, empurrando 15→22 para 16→23):** o documento já numera sequencialmente de 1 a 21 sem nenhuma seção "solta"; inserir no meio exigiria renumerar 7 headers de nível 2 e conferir se algum link interno aponta para esses números. Preferi um patch pequeno e de baixo risco a um logicamente "mais bonito" — mesmo critério, aliás, que me fez usar o `2.14` livre em vez de criar um `2.17` no fim da seção 2.

## 3. Como aplicar

```bash
git apply tarefa_doc_api_sumarios.patch
```

Já testado por Claude: `git apply --check` limpo e `git apply` produzindo exatamente o conteúdo esperado, num clone independente do GitHub.

## 4. Nota — o `spuripainel` mantém cópias deste mesmo arquivo, mas já estavam desatualizadas antes desta tarefa

Ao contrário da Tarefa 74 (onde a cópia do `spuripainel` estava só uma versão atrás), desta vez encontrei uma situação mais bagunçada, que **não tentei resolver neste patch**:

- Existem **duas** cópias em `spuripainel`: `src/Documentação da API.md` e `src/docs/Documentação da API.md`. Elas **não são idênticas entre si** (mais de 1300 linhas de diferença).
- Ambas estão na versão `2.3.0` — ou seja, já estavam **duas versões atrás** (faltando também o que a Tarefa 74 adicionou em `2.4.0`, não só Sumários em `2.5.0`).
- Ambas usam quebra de linha estilo Windows (CRLF), enquanto o arquivo no `spuri-backend` usa Unix (LF) — um patch gerado a partir de um não aplica limpo no outro sem conversão.

Isso não é algo para resolver "de passagem" dentro desta tarefa de documentação — decidir qual das duas cópias é a fonte da verdade (ou se `spuripainel` deveria parar de manter uma cópia própria e referenciar o arquivo do backend de alguma forma) é uma decisão de organização que vale uma tarefa própria. Deixo registrado aqui para você decidir o que fazer.

## 5. Checklist

1. `git apply tarefa_doc_api_sumarios.patch`.
2. Abrir o arquivo renderizado (preview do editor/GitHub) e conferir visualmente a nova seção `22. Sumários` e os trechos alterados em `14. Faltas`.
3. Mover este documento para `docs/Tarefas feitas/`.
4. (Opcional, decisão separada) Avaliar o que fazer com as cópias desatualizadas/divergentes em `spuripainel` — ver seção 4.
