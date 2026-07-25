# Depurar disponibilidade de todas as projeções para rebuild

## Objetivo do debug

Confirmar se todas as projeções registradas no bootstrap do servidor estão disponíveis para rebuild individual e participam da reconstrução geral em uma ordem determinística compatível com suas dependências. Caso alguma projeção estivesse fora da ordem explícita, a correção deveria incluí-la no pipeline e atualizar a documentação operacional.

## Arquivos auditados

- `cmd/server/main.go`
- `internal/projections/manager.go`
- `internal/projections/*_projection.go`
- `internal/projections/manager_rebuild_test.go`
- `Documentação.md`

## Resultado da auditoria

O bootstrap registra 12 projeções reconstruíveis:

1. `admins`
2. `academias`
3. `cursos`
4. `materias`
5. `categorias_nota`
6. `estudantes`
7. `turmas`
8. `notas`
9. `faltas`
10. `avaliacao_final`
11. `solicitacoes_matricula`
12. `solicitacoes_edicao_dados_estudante`

O rebuild individual já aceitava qualquer nome registrado no `Projection Manager`, portanto `solicitacoes_matricula` e `solicitacoes_edicao_dados_estudante` estavam acessíveis por `POST /dominis/projections/rebuild/:name`.

A lacuna encontrada estava no rebuild geral: a ordem explícita cobria apenas 10 projeções e deixava as duas projeções de solicitações para o fallback alfabético. Embora esse fallback ainda reconstruísse as projeções, ele escondia a dependência funcional e podia mascarar novas projeções esquecidas na ordem oficial.

## Correção aplicada

- Extraída a ordem padrão para `defaultRebuildOrder` em `internal/projections/manager.go`.
- Incluídas `solicitacoes_matricula` e `solicitacoes_edicao_dados_estudante` na ordem explícita, após `estudantes`/`turmas` e antes das projeções de notas/faltas/avaliação final.
- Criado `orderedRebuildProjectionNames`, que aplica a ordem de dependência conhecida e mantém fallback alfabético apenas para projeções futuras ainda não classificadas.
- Adicionados testes unitários para impedir que uma projeção registrada no servidor fique fora de `defaultRebuildOrder` e para validar que o fallback continua determinístico.
- Atualizada a documentação da seção de rebuild com a ordem completa.

## Checklist final

- [x] Todas as projeções registradas no servidor estão presentes em `defaultRebuildOrder`.
- [x] O rebuild individual continua protegido contra nomes não registrados.
- [x] O rebuild geral segue ordem determinística e completa antes de recorrer a fallback alfabético.
- [x] A documentação operacional lista `solicitacoes_matricula` e `solicitacoes_edicao_dados_estudante`.
- [x] `go test ./internal/projections` passa.
