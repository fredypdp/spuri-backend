# Auditoria — categorias de nota fixas e `materias_chave`

## Ocorrências corrigidas como bug ativo

- `internal/domain/aggregates/estudante_notas.go`: removidas as listas hardcoded de categorias escolares/superiores e a validação cruzada baseada em categorias fixas. A validação ativa agora exige somente que a categoria exista entre as categorias configuradas para a academia/ano acadêmico.
- `internal/domain/aggregates/curso.go`: criação de curso médio deixou de exigir `materias_chave`; o aggregate aceita criação sem configuração e mantém a validação estrutural somente quando `materias_chave` é configurado depois.
- `internal/handlers/cursos_handlers.go`: criação de curso rejeita `materias_chave` no payload; edição cadastral também rejeita esse campo; foi criada rota dedicada para configurar `materias_chave` após a criação das matérias.
- `cmd/server/main.go`: registrada a rota `PUT /academia/curso/:id/materias-chave`.
- `internal/projections/categorias_nota_projection.go`: `status` continua sendo detalhe interno de persistência/projeção para soft delete, mas foi removido do contrato JSON público de categoria de nota.

## Ocorrências classificadas como documentação a corrigir

- `docs/Spuri - Documentação.md`: removidas referências a categorias fixas/obrigatórias e atualizado o fluxo de `materias_chave` para configuração posterior por rota específica.
- `docs/Spuri - API.md`: removidas categorias fixas da seção de conceitos, removido `status` de `CategoriaNotaDTO`, removido `materias_chave` do request de criação de curso e documentada a rota específica de configuração.

## Ocorrências classificadas como teste a atualizar/adicionar

- `internal/domain/aggregates/curso_test.go`: adicionado teste garantindo criação de curso médio sem `materias_chave`.
- `internal/handlers/cursos_handlers_test.go`: adicionado teste garantindo rejeição de `materias_chave` na edição cadastral do curso.

## Ocorrências ainda válidas no novo modelo

- `projection_cursos.materias_chave`, DTOs de leitura e avaliação final continuam válidos: `materias_chave` é estado configurado do curso médio, usado pela avaliação final quando necessário.
- Mensagens e validações estruturais de `materias_chave` continuam válidas na rota específica: ano obrigatório, lista não vazia, duplicidades, curso/academia/ano da matéria e curso médio.
- `status` em tabelas/projeções internas permanece válido como mecanismo interno de ativação/inativação/soft delete, mas não é exposto no contrato público de categoria de nota.
- Exemplos de códigos de categoria em testes de fórmula são dados de teste cadastráveis, não listas aceitas implicitamente pelo backend.

## Ocorrências históricas/legadas aceitáveis

- Documentos em `docs/Debbugs/` e `docs/Tarefas feitas/` preservam histórico de tarefas anteriores. As documentações públicas (`docs/Spuri - Documentação.md` e `docs/Spuri - API.md`) foram atualizadas para o contrato atual.
