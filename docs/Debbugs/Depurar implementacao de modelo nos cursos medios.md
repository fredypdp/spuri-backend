# Depurar implementação de `modelo` nos cursos médios

## Objetivo da auditoria

Auditar criticamente a tarefa `docs/Lista de tarefas/Adicionar campo modelo aos cursos medios.md` e garantir que o campo público e persistente `modelo` foi implementado em todos os pontos necessários do fluxo de cursos.

Caso qualquer área esteja incompleta, inconsistente, sem validação, sem teste, sem documentação ou aceite silenciosamente payload inválido, esta depuração exige corrigir e concluir a implementação.

## Checklist executado

- Aggregate de curso: adicionados estado, eventos, apply/replay e validação de `modelo`.
- Handlers e DTOs: `modelo` aceito em criação/edição de cursos médios, obrigatório em criação e rejeitado para cursos superiores.
- Projeção: `projection_cursos` passa a persistir e expor `modelo` somente quando preenchido para curso médio.
- Migração: criada migração idempotente com coluna, constraint, índice parcial e backfill seguro para cursos médios existentes.
- Documentação: atualizados contratos da API e regras funcionais de cursos.
- Testes: cobertos os valores válidos, ausência/valor inválido em médio, rejeição em superior e atualização válida no aggregate.

## Correções aplicadas

- `modelo` é obrigatório para `type='medio'` e aceita somente `liceu` ou `tecnico`.
- `modelo` é case-sensitive e rejeita valores como `LICEU`, string vazia ou qualquer valor fora da enumeração.
- `type='superior'` rejeita `modelo` em criação e edição.
- Eventos `CursoCriado` e `CursoDadosAtualizados` preservam o campo para replay.
- A projeção grava `modelo` em coluna própria e omite o campo de JSON público quando vazio, evitando exposição em cursos superiores.
- A migração classifica cursos médios legados como `liceu` por padrão operacional inicial, documentado no comentário da coluna.

## Resultado

A tarefa foi concluída e protegida por testes automatizados nas camadas de domínio e handlers.
