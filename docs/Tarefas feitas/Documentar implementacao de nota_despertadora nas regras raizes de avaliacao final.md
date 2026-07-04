---
modificado: 2026-07-04 00:00
criado: 2026-07-04 00:00
---
# Documentar implementação de `nota_despertadora` nas regras raízes de avaliação final (feito)

## Objetivo

Documentar e, se necessário, complementar a implementação do campo `nota_despertadora` nas **regras raízes de avaliação final**.

Esse campo deve guardar o **código da categoria de nota** que desperta a avaliação final automática. Quando o estudante receber ou possuir nota lançada para essa categoria, o backend deve executar automaticamente a avaliação final aplicável para ele, respeitando o escopo da regra, o nível de ensino, as matérias avaliáveis, as regras descendentes, as pendências e a auditoria já existentes.

A documentação deve deixar explícito que `nota_despertadora` é um recurso exclusivo de regra raiz. Regras dependentes/descendentes não devem expor, aceitar, persistir nem executar lógica própria de despertar, porque elas já são ativadas automaticamente pela reprovação na regra ancestral correspondente.

## Contexto funcional

Atualmente a avaliação final automática depende de regras configuráveis por academia, nível, curso, ano académico, matérias aplicáveis e fórmula. A nova documentação precisa explicar como a execução automática é iniciada a partir de uma categoria de nota configurada na regra raiz.

A regra raiz representa o primeiro estágio da avaliação final automática. Ela observa a categoria configurada em `nota_despertadora` e, ao identificar que o estudante tem nota dessa categoria no contexto aplicável, dispara o cálculo da avaliação final para as matérias do escopo. As regras descendentes continuam sendo acionadas somente quando houver reprovação em uma etapa ancestral e nunca por lançamento direto da categoria despertadora.

## Contrato esperado

### Campo público

Adicionar e documentar o campo público:

```json
{
  "nota_despertadora": "codigo_da_categoria_de_nota"
}
```

Regras obrigatórias:

- `nota_despertadora` deve ser aceita apenas em regras raízes;
- `nota_despertadora` deve ser rejeitada em regras dependentes/descendentes;
- o valor deve ser o `codigo` de uma categoria de nota válida da academia autenticada;
- a categoria referenciada deve estar ativa e não deletada;
- a categoria deve pertencer ao mesmo contexto de nível/ano/período aplicável à regra, quando essa validação existir no modelo de categorias;
- o backend deve rejeitar código vazio, inexistente, duplicado de forma ambígua ou incompatível com a academia;
- respostas de criação, edição, detalhe e listagem de regras raízes devem expor `nota_despertadora`;
- respostas de regras descendentes não devem sugerir que o campo possa ser configurado nelas.

### Persistência e eventos

Validar e documentar que `nota_despertadora` é preservado em:

- payload público de criação de regra raiz;
- payload público de atualização de regra raiz;
- evento de criação/alteração de regra;
- modelo de domínio ou snapshot da regra;
- projeção de regras de avaliação final;
- respostas da API;
- replay de eventos;
- migrações e compatibilidade com regras existentes.

Regras existentes sem `nota_despertadora` devem ter comportamento explicitamente definido. Se o campo for obrigatório para novas regras raízes, a migração/compatibilidade deve explicar como tratar regras antigas. Se for opcional, a documentação deve explicar que regra sem categoria despertadora não dispara automaticamente por lançamento de nota e só executa por fluxo manual ou administrativo existente.

## Regras de ativação automática

Documentar e validar que a avaliação automática será executada para um estudante quando:

1. existir uma regra raiz ativa aplicável à academia, nível e escopo do estudante;
2. a regra raiz possuir `nota_despertadora` configurada;
3. o estudante possuir uma nota lançada para a categoria cujo `codigo` é igual a `nota_despertadora`;
4. a nota pertencer ao ano letivo/período/semestre e à matéria aplicável ao escopo da regra;
5. a avaliação ainda não tiver sido executada de forma idempotente para o mesmo contexto, ou a execução puder ser atualizada conforme regra já existente de recálculo.

Também documentar que:

- o gatilho deve ocorrer no fluxo de lançamento/atualização de notas;
- o gatilho deve respeitar idempotência para evitar avaliações duplicadas;
- a execução deve continuar usando a fórmula da regra raiz e os mecanismos atuais de seleção de matérias;
- regras descendentes devem ser chamadas apenas quando houver reprovação na etapa ancestral;
- uma nota de categoria despertadora não deve executar diretamente regra dependente;
- a ausência da categoria despertadora deve impedir apenas o disparo automático, não necessariamente a execução manual, se o produto mantiver endpoint manual.

## Escopo por tipo de regra

### Regra raiz

Para regra raiz:

- `nota_despertadora` pode ser criado, editado, lido e auditado;
- o campo define a categoria de nota que desperta a avaliação automática;
- a regra raiz continua responsável por iniciar a cadeia de avaliação;
- a execução da raiz pode acionar descendentes conforme reprovação por matéria.

### Regra descendente/dependente

Para regra descendente:

- `nota_despertadora` não deve estar disponível no contrato público;
- payload com `nota_despertadora` deve falhar com erro de validação claro;
- a resposta/documentação deve explicar que a descendente é acionada automaticamente pela reprovação na ancestral;
- não deve existir índice, busca, seleção ou executor que trate descendente como regra despertada por nota;
- descendente não deve concorrer com raiz na seleção de regra por categoria despertadora.

## Validações obrigatórias

Implementar ou confirmar as seguintes validações:

1. regra raiz sem categoria válida deve falhar, se `nota_despertadora` for obrigatório;
2. regra descendente com `nota_despertadora` deve falhar sempre;
3. código de categoria inexistente deve falhar;
4. categoria de outra academia deve falhar;
5. categoria deletada/inativa deve falhar;
6. categoria incompatível com o nível ou período da regra deve falhar quando aplicável;
7. alteração de regra raiz deve validar novamente a categoria despertadora;
8. ativação/re-ativação de regra raiz deve validar que a categoria ainda existe e está ativa;
9. replay/projeção não deve perder o campo;
10. listagem deve manter o campo em regras raízes sem vazar comportamento indevido para descendentes.

## Arquivos e áreas de referência obrigatória

Auditar e atualizar, no mínimo:

- `internal/handlers/avaliacao_final_regras.go`;
- `internal/handlers/avaliacao_final_handler.go`;
- `internal/handlers/notas_handlers.go`;
- `internal/handlers/avaliacao_final_regras_test.go`;
- `internal/handlers/notas_handlers_test.go`;
- `internal/projections/avaliacao_final_projection.go`;
- `internal/projections/avaliacao_final_projection_test.go`;
- `internal/projections/categorias_nota_projection.go`;
- `internal/domain/models.go`;
- `internal/domain/aggregates/estudante_avaliacao.go`;
- `internal/domain/aggregates/estudante_notas.go`;
- migrações relacionadas a `avaliacao_final_regras`;
- `docs/Spuri - Documentação.md`, seção **5.6 Avaliação Final de Ano Académico**;
- `docs/Spuri - API.md`, seção **15. Avaliações Finais**.

Não tratar essa lista como exaustiva. Fazer busca ampla por:

- `nota_despertadora`;
- `notaDespertadora`;
- `categoria_despertadora`;
- `categoriaDespertadora`;
- `avaliacao_final_regras`;
- `RegraAvaliacaoFinal`;
- `regra_pai_id`;
- `parent_rule_id`;
- `descendente`;
- `dependente`;
- `categoria_nota`;
- `codigo`.

Cada ocorrência relevante deve ser classificada como implementação correta, lacuna, compatibilidade legada, documentação histórica aceitável, teste de regressão ou bug a corrigir.

## Testes obrigatórios

Criar ou atualizar testes cobrindo, no mínimo:

### Contrato de regra

- cria regra raiz com `nota_despertadora` válida;
- rejeita regra raiz com `nota_despertadora` inexistente;
- rejeita regra raiz com categoria de outra academia;
- rejeita regra raiz com categoria inativa/deletada;
- atualiza `nota_despertadora` de regra raiz;
- lista/detalha regra raiz contendo `nota_despertadora`;
- rejeita criação de regra descendente com `nota_despertadora`;
- rejeita atualização de regra descendente com `nota_despertadora`.

### Execução automática

- lançamento de nota na categoria despertadora executa avaliação final raiz aplicável;
- lançamento de nota em outra categoria não executa avaliação final automática;
- estudante sem nota na categoria despertadora não é avaliado automaticamente;
- execução automática respeita escopo por nível, curso, ano académico, período e matérias aplicáveis;
- execução automática é idempotente no mesmo contexto;
- regra descendente é acionada por reprovação na raiz, não por nota despertadora;
- nota despertadora não dispara regra dependente diretamente.

### Persistência e replay

- evento de regra raiz preserva `nota_despertadora`;
- projeção preserva e expõe `nota_despertadora`;
- replay reconstrói regra raiz com `nota_despertadora`;
- migração mantém regras antigas em estado coerente;
- ativação/re-ativação valida novamente a categoria despertadora.

## Documentação obrigatória

Atualizar a documentação funcional e de API para explicar:

1. o que é `nota_despertadora`;
2. que o valor é o código da categoria de nota;
3. que o campo existe apenas em regra raiz;
4. que regras dependentes são disparadas automaticamente por reprovação ancestral;
5. quando a avaliação automática é executada;
6. exemplos de payload de criação/edição de regra raiz;
7. exemplo de erro ao enviar `nota_despertadora` em regra descendente;
8. comportamento de regras antigas sem o campo;
9. relação entre `nota_despertadora`, fórmula, matérias aplicáveis, pendências e regras descendentes;
10. garantias de idempotência e auditoria.

## Critérios de aceite

A tarefa só pode ser considerada concluída quando:

- `nota_despertadora` estiver documentado no contrato público de regras raízes;
- regras descendentes rejeitarem o campo de forma clara;
- o fluxo de lançamento/atualização de nota disparar automaticamente a avaliação final quando a categoria corresponder ao código configurado;
- a execução automática respeitar escopo, idempotência e auditoria;
- eventos, projeções, respostas e replay preservarem o campo;
- testes cobrirem contrato, execução automática, persistência e regressões;
- `docs/Spuri - Documentação.md` e `docs/Spuri - API.md` estiverem atualizados com exemplos e erros;
- não existir comportamento silencioso em que regra dependente seja despertada diretamente por nota.
