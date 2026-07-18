---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Índice e ordem de implementação das tarefas pendentes

## Objetivo deste documento

Este índice organiza, em ordem recomendada de implementação, as tarefas derivadas de `Lista de tarefas.md` (seções "Tarefas do Back-End" e "Situações que devemos prever"). Cada item foi transformado num documento individual, seguindo o mesmo padrão estrutural dos arquivos em `docs/Tarefas feitas/` — especialmente o padrão de `docs/Tarefas feitas/Adicionar nif alvara limite pdf e padronizar erros.md` — com Prompt recomendado, Contexto, Resumo executivo, seções numeradas de Objetivo/Regra de negócio/Escopo obrigatório, Fora de escopo e Critérios de aceite. Cada tarefa foi escrita para ser executável de forma independente por um desenvolvedor ou por uma IA sem contexto adicional além de `Documentação.md` e do próprio arquivo da tarefa.

Itens de contexto semelhante foram agrupados numa única tarefa, conforme solicitado, em vez de gerar arquivos duplicados ou sobrepostos.

## Critério de priorização

A ordem abaixo **não** segue a ordem de aparição em `Lista de tarefas.md`. Ela foi recalculada por criticidade, definida como:

- **Nível 1 — Crítico**: lacunas que colocam em risco a integridade dos dados, a corretude de decisões acadêmicas já automatizadas (avaliação final, finalização de ano letivo) ou que permitem estados inconsistentes em fluxos que já estão em produção (matrícula duplicada). Afetam o funcionamento correto do que já existe.
- **Nível 2 — Alto**: revisões e reforços de funcionalidades essenciais já existentes (eventos de progressão do estudante, edição de dados cadastrais). O sistema funciona sem elas, mas fica exposto a inconsistências operacionais que tendem a aparecer com o uso contínuo.
- **Nível 3 — Médio**: novas capacidades administrativas que agregam valor operacional, mas cuja ausência não compromete o funcionamento atual do sistema.
- **Nível 4 — Baixo**: funcionalidades novas de maior porte, funcionalidades dependentes de outra tarefa ainda não implementada, e trabalho de documentação sem impacto funcional direto.

Dentro de cada nível, a ordem numérica indica a sequência sugerida. A ordem **entre** níveis é rígida: nenhuma tarefa de um nível inferior deve ser priorizada sobre uma tarefa de nível superior sem justificativa explícita registrada pela equipe.

## Tarefas por ordem de implementação

### Nível 1 — Crítico

| # | Tarefa | Arquivo | Resumo |
|---|---|---|---|
| 1 | Validar e reforçar a integridade do event sourcing e dos rebuilds | `01 - Validar e reforçar a integridade do event sourcing e dos rebuilds.md` | Comprova por teste que o ledger realmente **impede** adulteração (não apenas detecta) e que os rebuilds de projeções são idempotentes e seguros contra ledger corrompido. |
| 2 | Padronizar avaliação final diante de notas ausentes ou alteradas | `02 - Padronizar avaliação final diante de notas ausentes ou alteradas.md` | Define o comportamento quando o gatilho da avaliação final dispara mas falta alguma nota exigida pela fórmula, e registra a regra de negócio para quando notas puderem ser corrigidas no futuro. |
| 3 | Bloquear finalização de ano letivo fora do ano final do período | `03 - Bloquear finalização de ano letivo fora do ano final do período.md` | Fecha uma lacuna em `POST /academia/anos-letivos/finalizar`, que hoje valida apenas o mês da janela de finalização, não o ano. |
| 4 | Prevenir e sinalizar matrícula duplicada em múltiplas instituições | `04 - Prevenir e sinalizar matrícula duplicada em múltiplas instituições.md` | Adiciona verificação de solicitações semelhantes na mesma academia e cancelamento automático de solicitações concorrentes de outras academias quando uma é aprovada. |

### Nível 2 — Alto

| # | Tarefa | Arquivo | Resumo |
|---|---|---|---|
| 5 | Revisar e ampliar eventos de progressão e status acadêmico do estudante | `05 - Revisar e ampliar eventos de progressão e status acadêmico do estudante.md` | Audita os endpoints de acontecimentos já existentes (matrícula, interrupção, trancamento, desvínculo, revínculo) e adiciona um evento de ajuste administrativo para corrigir ano acadêmico/curso/semestre do estudante. |
| 6 | Reforçar validações na edição de dados cadastrais dos usuários | `06 - Reforçar validações na edição de dados cadastrais dos usuários.md` | Corrige inconsistências entre `PUT /academia/dados`, `PUT /estudante/dados-pessoais` e `PUT /dominis/admin/:id/dados`, incluindo o fechamento imediato de uma brecha que permite alterar `anos_academicos`, `cursos`, `type` e `nivel_escolar` da academia sem as validações já existentes em rotas dedicadas. |

### Nível 3 — Médio

| # | Tarefa | Arquivo | Resumo |
|---|---|---|---|
| 7 | Permitir academia alterar `type` e `nivel_escolar` mediante documento comprobativo | `07 - Permitir academia alterar type e nível escolar mediante documento.md` | Cria fluxo dedicado, com upload de documento comprobativo e validação de impacto em dados dependentes, para mudança formal de `type` e `nivel_escolar`, substituindo o caminho fechado na tarefa 6. |
| 8 | Criar fluxo de atualização de documentos de estudantes e academias com aprovação | `08 - Fluxo de atualização de documentos de estudantes e academias com aprovação.md` | Nova entidade `SolicitacaoAtualizacaoDocumento`, com armazenamento temporário do novo arquivo até aprovação (pela academia, no caso do estudante; por um Admin, no caso da academia). |

### Nível 4 — Baixo

| # | Tarefa | Arquivo | Resumo |
|---|---|---|---|
| 9 | Implementar reprovação por falta com limite percentual configurável | `09 - Reprovação por falta com limite percentual configurável.md` | Depende da funcionalidade de sumários/aulas, que já possui tarefa própria fora deste índice (ver observação abaixo). Fica pronta para implementação assim que o pré-requisito existir. |
| 10 | Implementar gabarito de prova com digitalização e correção automática | `10 - Gabarito de prova com digitalização e correção automática.md` | Nova funcionalidade de correção automática de provas objetivas, dividida em fase de modelo de dados/comparação e fase de digitalização/OCR. |
| 11 | Enriquecer a documentação da API com o significado de cada campo | `11 - Enriquecer documentação da API com significado de cada campo.md` | Trabalho de documentação pura, sem impacto funcional; pode ser feito a qualquer momento sem risco. |

## Item intencionalmente não transformado em tarefa

O item "[[Adicionar sumários e vincular opcionalmente às faltas]]" citado em `Lista de tarefas.md` **não** foi transformado num novo arquivo de tarefa porque o próprio documento de origem informa explicitamente que já existe uma tarefa criada para ele ("já existe uma tarefa criada para ele"). Criar um segundo arquivo divergente para o mesmo escopo contrariaria a orientação de não duplicar tarefas com contexto semelhante. A tarefa 9 deste índice (reprovação por falta) depende dessa funcionalidade e referencia essa dependência explicitamente em vez de repeti-la.

## Observações gerais

- Todas as tarefas seguem o princípio de não deixar código legado, aliases, wrappers de compatibilidade ou fallbacks temporários, conforme o padrão já usado nas tarefas concluídas do repositório.
- Toda tarefa que altera contrato público exige atualização de `Documentação.md`, do OpenAPI/Swagger (quando existir) e de testes automatizados, seguindo o mesmo padrão das tarefas já concluídas.
- Todas as ações administrativas novas (academia sobre estudante, admin sobre academia) devem continuar seguindo o padrão de event sourcing já estabelecido: nenhuma mudança de estado sensível deve ser feita por atualização direta de campo sem evento correspondente gravado no ledger.
- Ao concluir cada tarefa, siga o "Procedimento de conclusão" descrito no final de cada arquivo e mova o documento para `docs/Tarefas feitas/`.
