---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Enriquecer documentação da API com significado de cada campo (pendente)

## Prompt recomendado para executar a atualização

Revise `Documentação.md` de ponta a ponta, especialmente a seção "2. Estruturas de Dados", e adicione, para cada campo de cada interface/DTO do sistema, uma descrição clara do seu significado, propósito de negócio e restrições — não apenas o tipo TypeScript já existente. Ao final, garanta que nenhuma estrutura de dados do documento tenha campo sem explicação associada. Esta tarefa é exclusivamente de documentação e não deve alterar nenhum comportamento do backend.

## Contexto

`Documentação.md` já descreve, na seção "2. Estruturas de Dados", todas as interfaces do sistema (`AdminDTO`, `AcademiaDTO`, `EstudanteDTO`, `SolicitacaoMatriculaDTO`, `CursoDTO`, `MateriaDTO`, `TurmaDTO`, `NotaDTO`, `FaltaDTO`, `NotaRegistroDTO`, `FaltaRegistroDTO`, `AvaliacaoFinalDTO`, `CategoriaNotaDTO`, `JobSummary`/`JobDetail`/`JobItemResult`, `AsyncBatchAcceptedResponse`) no formato de interfaces TypeScript com comentários inline breves. Vários campos possuem apenas o tipo e um exemplo (ex.: `ano_superior?: string // ex: '1_ano_superior'`), sem explicar seu papel dentro do domínio de negócio, quando ele é preenchido, quando é `null`/ausente, e como se relaciona com outros campos da mesma entidade ou de entidades vizinhas.

`Lista de tarefas.md` pede explicitamente: "Atualizar documentação da API: Adicionar o significado/função de cada campo de cada estrutura/entidade do sistema." Esta tarefa é puramente documental — não há necessidade de alterar código, rodar suíte de testes Go ou tocar em qualquer contrato já existente. O objetivo é que qualquer pessoa (ou IA) lendo `Documentação.md` consiga entender o papel de cada campo sem precisar inferir a partir de outras seções ou do próprio código-fonte.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Formato | Tabela de campos (`Campo \| Tipo \| Obrigatório \| Significado`) logo abaixo de cada interface | Padronização visual e fácil leitura por entidade |
| Cobertura | Toda interface da seção 2 de `Documentação.md` | Nenhum campo sem descrição de significado |
| Fonte da verdade | Comportamento já implementado, não comportamento desejado | Nenhuma descrição deve prometer comportamento que o código não suporta |
| Consistência | Reaproveitar a terminologia já usada nas seções de processos/regras de negócio | Nenhuma descrição nova contradiz o restante do documento |

---

# 1. Adicionar tabela de significado de campos por entidade

## Objetivo

Garantir que cada interface da seção "2. Estruturas de Dados" de `Documentação.md` tenha, imediatamente após o bloco de código TypeScript, uma tabela explicando o significado de cada campo.

## Regra de negócio

Para cada interface listada, adicionar uma tabela no formato:

```markdown
| Campo | Tipo | Obrigatório | Significado |
| --- | --- | --- | --- |
| `codigo_estudante` | string | Sim | Código público gerado pelo backend no formato `AAA1234`, usado como identificador externo do estudante em todas as rotas e como parte da senha inicial de acesso. |
```

A coluna "Obrigatório" deve refletir a obrigatoriedade **no contexto em que o campo é preenchido** (ex.: um campo pode ser opcional na criação, mas sempre presente na resposta depois de preenchido — isso deve ser explicado na própria célula de significado quando relevante, não apenas com "Sim"/"Não").

## Escopo obrigatório

### 1.1 Entidades a cobrir obrigatoriamente

No mínimo, todas as interfaces já existentes na seção 2 de `Documentação.md`:

1. `AdminDTO`;
2. `AcademiaDTO` e `AnoLetivoItem`;
3. `EstudanteDTO`;
4. `SolicitacaoMatriculaDocumentoDTO` e `SolicitacaoMatriculaDTO`;
5. `CursoDTO`;
6. `MateriaDTO`;
7. `TurmaDTO`;
8. `NotaDTO`;
9. `FaltaDTO`;
10. `NotaRegistroDTO`;
11. `FaltaRegistroDTO`;
12. `AvaliacaoFinalDTO`;
13. `CategoriaNotaDTO`;
14. `JobSummary`, `JobDetail`, `JobItemResult`;
15. `AsyncBatchAcceptedResponse`.

Se, no momento da execução desta tarefa, novas interfaces tiverem sido adicionadas por outras tarefas já concluídas (por exemplo, `SolicitacaoAtualizacaoDocumento` ou `Gabarito`, se as tarefas 08 e 10 deste conjunto já estiverem implementadas), elas também devem ser cobertas.

### 1.2 Campos que dependem de outros campos

Para campos cujo significado só faz sentido em relação a outro campo da mesma entidade (ex.: `ano_superior` só é preenchido quando o estudante está vinculado ao ensino superior; `curso_medio_id` só é relevante quando `ano_escolar_medio` está preenchido), a descrição deve deixar essa dependência explícita, evitando que o leitor precise inferir a relação.

### 1.3 Campos reservados/não funcionais

Campos já marcados como reservados para funcionalidade ainda não ativa (ex.: `telefone_verificado`, `telefone_encarregado_verificado`) devem ter isso reafirmado na coluna de significado, reaproveitando a mesma ressalva já usada em `docs/Tarefas feitas/Remoção do número de telefone extra.md`: "reservado; verificação ainda não implementada."

### 1.4 Revisão cruzada com as seções de processo de negócio

Depois de escrever as tabelas, revisar se alguma descrição contradiz o que já está descrito nas seções de "Processos de Negócio" e "Regras de Negócio" de cada domínio (seções 6 a 20 de `Documentação.md`). Qualquer contradição encontrada deve ser corrigida antes de finalizar, priorizando o comportamento real já documentado nessas seções.

---

# 2. Verificações obrigatórias antes de finalizar

Como esta é uma tarefa exclusivamente de documentação, a verificação final não exige suíte de testes Go, mas exige:

1. releitura de cada tabela criada contra a interface TypeScript correspondente, confirmando que todos os campos da interface têm uma linha na tabela;
2. busca textual por termos centrais do domínio (`ano_academico`, `status_escolar`, `ano_superior`, `semestre_atual`, `documentos`, `nota_despertadora`, `pendencia_permitida`) para confirmar que a terminologia usada nas novas tabelas é a mesma já usada no restante do documento;
3. confirmação de que nenhuma tabela nova promete comportamento que as seções de regras de negócio não confirmam como já implementado.

---

# Fora de escopo

- Alterar qualquer contrato de API, campo, endpoint ou comportamento do backend.
- Criar OpenAPI/Swagger a partir do zero, caso ainda não exista formalmente (isso seria uma tarefa própria, não esta).
- Traduzir a documentação para outro idioma.
- Reescrever as seções de "Processos de Negócio" e "Regras de Negócio" além do necessário para corrigir contradições encontradas na seção 1.4.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. todas as interfaces listadas na seção 1.1 tiverem uma tabela de significado de campos imediatamente após o bloco de código TypeScript correspondente;
2. nenhum campo de nenhuma interface listada estiver sem descrição de significado;
3. campos dependentes de outros campos tiverem essa relação explicitada;
4. campos reservados/não funcionais estiverem claramente identificados como tal;
5. nenhuma descrição nova contradizer o que já está documentado nas seções de processo/regras de negócio;
6. nenhuma alteração de comportamento do backend tiver sido feita como parte desta tarefa.

## Procedimento de conclusão

Ao finalizar a atualização:

1. atualizar o título interno desta tarefa para `# Enriquecer documentação da API com significado de cada campo (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
