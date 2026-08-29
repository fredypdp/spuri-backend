---
criado: 2026-08-29
status: pronto_para_execucao
tipo: documentacao
patch: tarefa_doc_api_delecao.patch
depende_de: "Tarefa 74 (já aplicada) — este documento só atualiza a Documentação da API.md para refletir o que já está no ar"
---

# Atualizar `Documentação da API.md` com os endpoints da Tarefa 74 (deleção auditável)

## 0. O que aconteceu

A Tarefa 74 (mecanismo de deleção auditável para Academia/Administrador/Estudante) foi aplicada e mesclada, mas a atualização da `Documentação da API.md` — que é prática padrão neste repositório para mudanças de contrato de API (ver Tarefas 48 e 51, que fizeram o mesmo) — ficou pendente. Este documento cobre só isso: é puro markdown, sem migration, sem Go, sem risco de regressão de código. Claude conferiu a estrutura do arquivo inteiro depois da edição (blocos de código balanceados, numeração de subseções consistente, âncoras de link corrigidas) e validou que o patch aplica limpo num clone independente e recente de `main`.

## 1. Prompt recomendado

> Aplique `tarefa_doc_api_delecao.patch` na raiz do repositório (`git apply tarefa_doc_api_delecao.patch`) e confirme visualmente que o markdown renderiza corretamente (sem blocos de código quebrados, sem headers duplicados). Não precisa rodar `go build`/testes — este patch toca só `Documentação da API.md`. Depois, mova este documento para `docs/Tarefas feitas/` seguindo a convenção normal.

## 2. O que o patch muda em `Documentação da API.md`

| Seção | Mudança |
|---|---|
| Frontmatter | `modificado: 29-08-2026`, versão `2.3.0 → 2.4.0` |
| `DELETE /dominis/academia/:codigo` | descrição atualizada com a regra de estudantes vinculados; novo erro `409` |
| `GET /academias` | nota: nunca retorna `status = deletado`; aponta para o endpoint de auditoria |
| `GET /estudantes` | mesma nota |
| `GET /dominis/admin-lista` | mesma nota |
| **`### 8.3 Autodeleção de conta`** (nova) | documenta `DELETE /estudante/conta` |
| **`#### DELETE /dominis/admin/:id`** (nova) | inserida logo após `PUT /dominis/admin/:id/desativar`, mesmo formato |
| **`### 16.6 Auditoria de Deleções`** (nova) | documenta `GET /dominis/auditoria/delecoes` (query params, os 3 formatos de item na resposta, erros) |

Todas as entradas novas seguem exatamente o formato já usado no resto do arquivo (`**Proteção**:`, `**Request:**`, `**Response 200:**`, `**Erros:**` em bullet list) — conferido contra `PUT /dominis/admin/:id/desativar` como referência antes de escrever.

## 3. Como aplicar

```bash
git apply tarefa_doc_api_delecao.patch
```

Já testado por Claude: `git apply --check` limpo e `git apply` produzindo exatamente o conteúdo esperado, num clone independente do GitHub.

## 4. Nota — o `spuripainel` mantém uma cópia deste mesmo arquivo

`spuripainel/src/docs/Documentação da API.md` parece ser uma cópia sincronizada manualmente deste arquivo (mesmo conteúdo, mesma versão `2.3.0` na última checagem de Claude). Se o fluxo de trabalho é copiar este arquivo para lá depois de atualizado aqui, não esqueça desse passo — não está incluído neste patch (é um repositório diferente).

## 5. Checklist

1. `git apply tarefa_doc_api_delecao.patch`.
2. Abrir o arquivo renderizado (preview do editor/GitHub) e conferir visualmente as 3 seções novas.
3. Mover para `docs/Tarefas feitas/`.
