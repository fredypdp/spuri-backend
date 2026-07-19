---
criado: 2026-07-19 00:00
modificado: 2026-07-19 00:00
---
# Depurar renomeação de responsável para encarregado

Tarefa: [[12 - Renomear campos de responsável para encarregado]]

## Objetivo da auditoria

Auditar a implementação da tarefa `docs/Tarefas feitas/12 - Renomear campos de responsável para encarregado.md`, confirmar que o contrato público usa exclusivamente `encarregado` e corrigir qualquer aceite silencioso dos nomes removidos `responsavel`.

## Busca ampla executada

Comando usado na auditoria:

```bash
rg -n "respons[aá]vel|Responsavel|responsavel|bi_responsavel|telefone_responsavel|bilhete_identidade_responsavel" --glob '!docs/Tarefas feitas/**' --glob '!bash.exe.stackdump' .
```

## Classificação das ocorrências relevantes

| Ocorrência | Classificação | Resultado |
| --- | --- | --- |
| `migrations/092_renomear_responsavel_para_encarregado.sql` | Migration/backfill obrigatório para colunas e metadados antigos em projeções reconstruíveis. | Correto manter os nomes antigos apenas como origem de migração. |
| `internal/projections/*_projection.go` com `TelefoneResponsavel`, `BilheteIdentidadeResponsavel` e `bi_responsavel` | Interpretação isolada de eventos históricos imutáveis durante rebuild. | Correto manter no aplicador de projeção, com saída normalizada para `encarregado`. |
| `internal/handlers/removed_fields.go` | Rejeição explícita de campos removidos em contratos públicos. | Correto e ampliado nesta depuração. |
| `internal/handlers/batch_handlers.go` | Endpoint público de cadastro direto em lote. | Bug encontrado: JSON do lote e JSON textual `estudantes` no multipart podiam ignorar campos removidos silenciosamente. Corrigido. |
| `internal/handlers/estudante_handlers.go` em `CompletarDocumentosEstudantePendente` | Endpoint público de upload posterior de documentos. | Bug encontrado: arquivo `bi_responsavel` era rejeitado genericamente como campo inválido, sem mensagem contratual de campo removido. Corrigido. |

## Correções aplicadas

1. `POST /academia/estudante/register/async` em modo JSON agora executa a rejeição recursiva de campos removidos antes do bind do payload.
2. `POST /academia/estudante/register/async` em modo multipart agora inspeciona o JSON textual do campo `estudantes` antes de desserializar para DTO, rejeitando `telefone_responsavel`, `bilhete_identidade_responsavel` e demais nomes removidos mesmo quando aninhados por estudante.
3. `POST /academia/estudante/{codigo_estudante}/documentos` agora usa a mesma rejeição explícita de multipart para retornar `campo_removido` quando receber `bi_responsavel`.
4. Foram adicionados testes unitários para a detecção de campos removidos em JSON aninhado, JSON textual do multipart e uploads de arquivo com nome removido.

## Validação

- A implementação ativa permanece usando `telefone_encarregado`, `bilhete_identidade_encarregado`, `telefone_encarregado_verificado` e `bi_encarregado` como contrato público.
- As únicas ocorrências de `responsavel` fora da documentação histórica ficam restritas a migration/backfill, interpretação interna de eventos históricos e rejeição explícita de campos removidos.
- A suíte Go não pôde ser concluída neste ambiente porque o módulo `github.com/t3rm1n4l/go-mega` não está disponível no `go.mod`, bloqueando a compilação dos pacotes que importam `internal/storage`.
