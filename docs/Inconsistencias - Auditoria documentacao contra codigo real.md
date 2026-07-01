# Inconsistências encontradas — auditoria documentação contra código real

## `docs/Spuri - API.md`

| Secção afetada | Item incorreto | Documentado antes | Código real demonstrou | Correção aplicada | Fonte da verdade |
|---|---|---|---|---|---|
| Secção inicial `Telefones nativos` | Secção operacional autônoma de telefones nativos | Havia uma secção própria antes de `Convenções Globais` descrevendo contrato de telefones nativos. | O backend mantém campos nativos de telefone nas entidades, mas não possui endpoint/contrato público autônomo de telefone nativo. | Secção removida; campos reais de telefone foram preservados nas entidades. | `cmd/server/main.go`; `internal/projections/admin_projection.go`; `internal/projections/academia_projection.go`; `internal/projections/estudante_projection.go` |
| `Convenções Globais` / `Envelope de Erro` | Padronização universal de erro | A documentação dizia que todas as respostas de erro tinham `error`, `message`, `request_id` e `details`. | Middlewares e alguns handlers retornam erro simples com apenas `error`; o envelope completo existe nas rotas que usam `utils.RespondWithError`/`RespondWithDetailedError`. | Texto ajustado para distinguir envelope padronizado e erros simples legados. | `internal/utils/errors.go`; `internal/middleware/auth.go`; `internal/middleware/admin_auth_middleware.go`; `internal/handlers/batch_handlers.go` |
| `Estruturas de Dados` / `AdminDTO` | `updated_at` e `version` | `updated_at` estava opcional e `version` não estava documentado. | `AdminDTO` serializa `updated_at` e `version` sempre. | `updated_at` marcado como obrigatório e `version` adicionado. | `internal/projections/admin_projection.go` |
| `Estruturas de Dados` / `AcademiaDTO` | `telefone_verificado` | Campo documentado como retornado em academia. | `AcademiaDTO` não possui `telefone_verificado`. | Campo removido de `AcademiaDTO`. | `internal/projections/academia_projection.go` |
| `Estruturas de Dados` / `EstudanteDTO` | `documentos` | Campo público não estava documentado na estrutura base. | `EstudanteDTO` serializa `documentos,omitempty`. | Campo `documentos?: Record<string, SolicitacaoMatriculaDocumentoDTO>` adicionado. | `internal/projections/estudante_projection.go`; `internal/domain/aggregates/solicitacao_matricula.go` |
| `Turmas` / rota de remover estudante | Parâmetro de path | A rota estava documentada como `:codigoEstudante`. | A rota registrada usa `:codigo_estudante`. | Path documentado corrigido para `DELETE /academia/turma/:codigo/estudantes/:codigo_estudante`. | `cmd/server/main.go` |

## `docs/Spuri - Documentação.md`

| Secção afetada | Item incorreto | Documentado antes | Código real demonstrou | Correção aplicada | Fonte da verdade |
|---|---|---|---|---|---|
| `4.7 Telefone Extra` | Entidade/contrato removido | A documentação descrevia telefone extra, normalização e eventos `TelefoneExtraAdicionado`/`TelefoneExtraVerificado`. | O backend atual não registra rotas de telefone extra e a migração `068_remocao_telefone_extra_campos_nativos.sql` removeu esse modelo legado. | Secção removida. | `cmd/server/main.go`; `migrations/068_remocao_telefone_extra_campos_nativos.sql` |
| `Telefones nativos e remoção de telefone extra` | Secção autônoma obsoleta | Havia uma secção própria de telefones nativos/remoção de telefone extra. | A tarefa solicitou remoção da secção `Telefones nativos`; os campos reais permanecem documentados nas entidades. | Secção removida sem remover campos reais de telefone nas entidades. | `internal/projections/admin_projection.go`; `internal/projections/academia_projection.go`; `internal/projections/estudante_projection.go` |
