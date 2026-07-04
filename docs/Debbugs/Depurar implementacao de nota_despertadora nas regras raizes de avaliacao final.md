---
modificado: 2026-07-04 00:00
criado: 2026-07-04 00:00
---
# Depurar implementação de `nota_despertadora` nas regras raízes de avaliação final

Tarefa: [[Documentar implementacao de nota_despertadora nas regras raizes de avaliacao final]]

## Objetivo da auditoria

Fazer uma auditoria crítica, completa e arquivo por arquivo da implementação da tarefa:

`docs/Debbugs/Documentar implementacao de nota_despertadora nas regras raizes de avaliacao final.md`

A auditoria deve confirmar se a implementação foi feita corretamente, completamente e **à risca**. Caso qualquer parte esteja incompleta, inconsistente, parcialmente implementada, sem validação, sem teste, sem documentação, com comportamento silencioso incorreto ou divergente do contrato esperado, esta tarefa exige **instruir o ajuste e implementar o que falta**.

Esta funcionalidade é crítica porque `nota_despertadora` define a categoria de nota que inicia automaticamente a cadeia de avaliação final. O backend deve aceitar, persistir, expor e executar esse campo apenas em **regras raízes**, enquanto regras dependentes/descendentes continuam sendo acionadas exclusivamente pela reprovação na etapa ancestral correspondente.

## Regra adicional obrigatória

Além da especificação original, é obrigatório garantir que:

- a implementação real não trate `nota_despertadora` como campo meramente documental;
- o lançamento ou atualização de nota na categoria configurada realmente dispare a avaliação final automática da regra raiz aplicável;
- regras descendentes rejeitem `nota_despertadora` em qualquer payload público e nunca sejam selecionadas diretamente por categoria despertadora;
- a ausência de `nota_despertadora` em dados legados tenha comportamento explícito, documentado e testado;
- idempotência, auditoria, pendências, fórmulas, matérias aplicáveis e regras descendentes continuem funcionando sem regressão;
- ao concluir a auditoria e as correções, o título desta tarefa deve receber o sufixo ` (feito)` e o arquivo deve ser movido para `docs/Tarefas feitas`.

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. contratos públicos, DTOs e validações de criação, edição, leitura, listagem, ativação e inativação de regras de avaliação final;
2. handlers de lançamento e atualização de notas que podem iniciar avaliação final automática;
3. executor/serviço/handler de avaliação final automática;
4. seleção de regra ativa por academia, nível, curso, ano académico, período, matéria e categoria despertadora;
5. modelos de domínio, aggregates, eventos e snapshots de regras e avaliações finais;
6. projeções de regras de avaliação final e categorias de nota;
7. migrations, schemas, constraints, defaults e índices relacionados a `avaliacao_final_regras`;
8. replay de eventos e reconstrução de projeções;
9. validação de categorias de nota por academia, estado ativo/deletado e compatibilidade de contexto;
10. testes unitários, integração/handler, projeção, migração e regressão;
11. documentação funcional e documentação de API;
12. qualquer fluxo alternativo que persista, consuma, copie, liste ou execute regra de avaliação final.

Também auditar, no mínimo, os arquivos citados na tarefa original:

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

## Checklist obrigatório de validação

### 1. Busca ampla e classificação de ocorrências

Fazer busca ampla no repositório por:

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

Não basta listar ocorrências. Cada ocorrência relevante deve ser classificada como:

- implementação correta do novo contrato;
- compatibilidade/migração legada necessária e documentada;
- documentação histórica aceitável;
- bug ativo a corrigir;
- teste cobrindo regressão;
- código morto a remover.

### 2. Contrato público de regras raízes

Confirmar e, se necessário, implementar que regras raízes aceitam e expõem `nota_despertadora` no formato:

```json
{
  "nota_despertadora": "codigo_da_categoria_de_nota"
}
```

Validar que:

- criação de regra raiz aceita `nota_despertadora` válida;
- edição de regra raiz permite alterar `nota_despertadora`;
- detalhe de regra raiz retorna `nota_despertadora`;
- listagem de regras raízes retorna `nota_despertadora`;
- eventos, snapshots e projeções preservam o campo;
- replay reconstrói o campo sem perda de dados;
- regras antigas sem o campo têm comportamento explícito e documentado;
- o campo não é renomeado, omitido ou exposto apenas parcialmente entre request, response, evento e projeção.

### 3. Rejeição obrigatória em regras descendentes

Confirmar e, se necessário, implementar que regras dependentes/descendentes:

- rejeitam `nota_despertadora` na criação;
- rejeitam `nota_despertadora` na edição;
- não expõem o campo como configurável em documentação ou exemplos;
- não são indexadas, buscadas nem executadas diretamente por categoria despertadora;
- continuam sendo executadas somente por reprovação na regra ancestral;
- retornam erro claro, determinístico e auditável quando o campo for enviado indevidamente.

### 4. Validação da categoria de nota

Confirmar que o valor de `nota_despertadora` é sempre validado como `codigo` de categoria de nota da academia autenticada.

Validar rejeição clara para:

- código vazio;
- código inexistente;
- código ambíguo ou duplicado quando o modelo permitir ambiguidade;
- categoria de outra academia;
- categoria inativa;
- categoria deletada ou soft-deleted;
- categoria incompatível com o nível, ano, período, curso ou escopo da regra quando esse vínculo existir;
- alteração ou reativação de regra raiz cujo código configurado deixou de ser válido.

### 5. Persistência, migração e replay

Auditar migrations e schema para garantir que:

- `nota_despertadora` é persistido em `avaliacao_final_regras` ou estrutura equivalente;
- o tipo, default e nulabilidade refletem o contrato adotado para dados novos e legados;
- a migração é idempotente conforme o padrão do projeto;
- constraints e índices não permitem estado incoerente quando houver regra raiz com categoria inválida;
- replay de eventos antigos sem o campo não quebra;
- replay de eventos novos preserva o campo;
- não existe fallback silencioso que invente categoria despertadora para regra antiga.

### 6. Execução automática por lançamento de nota

Confirmar e, se necessário, implementar que o fluxo de lançamento/atualização de notas:

1. identifica a academia, estudante, nível, curso, ano académico/período, matéria e categoria da nota;
2. procura regra raiz ativa aplicável ao mesmo contexto;
3. compara o `codigo` da categoria da nota com `nota_despertadora` da regra raiz;
4. executa a avaliação final automática somente quando houver correspondência;
5. usa a fórmula, matérias aplicáveis, pendências e auditoria já existentes;
6. aciona regras descendentes apenas quando houver reprovação na etapa ancestral;
7. não dispara avaliação final para nota de categoria diferente;
8. não dispara regra descendente diretamente por nota despertadora;
9. respeita idempotência para evitar avaliações duplicadas no mesmo contexto;
10. registra auditoria suficiente para explicar qual nota/categoria disparou a avaliação.

### 7. Escopo e seleção de regras

Confirmar que a seleção da regra despertada respeita:

- academia autenticada;
- nível de ensino;
- curso quando aplicável;
- ano académico, período ou semestre quando aplicável;
- matéria avaliada;
- matérias aplicáveis configuradas na regra;
- status ativo da regra;
- regra raiz versus descendente;
- compatibilidade com regras legadas sem `nota_despertadora`.

Também confirmar que não existe conflito silencioso quando mais de uma regra raiz ativa poderia ser despertada pela mesma nota no mesmo contexto. Se o produto permitir múltiplas regras por escopo, a ordem/critério de seleção deve ser explícito, determinístico, testado e documentado.

### 8. Auditoria, snapshots e idempotência

Confirmar que avaliações finais geradas automaticamente registram, quando o modelo permitir:

- regra raiz utilizada;
- `nota_despertadora` configurado na regra;
- categoria e nota que dispararam a execução;
- estudante, matéria, curso e ano/período avaliados;
- resultados por matéria;
- regras descendentes acionadas;
- pendências criadas;
- motivo para não reexecutar quando a avaliação já existir no mesmo contexto.

A alteração posterior da regra ou categoria não deve apagar a explicação histórica da avaliação já registrada.

### 9. Testes obrigatórios

Criar ou ajustar testes cobrindo, no mínimo:

#### Contrato de regra

- criação de regra raiz com `nota_despertadora` válida;
- rejeição de regra raiz com `nota_despertadora` inexistente;
- rejeição de regra raiz com categoria de outra academia;
- rejeição de regra raiz com categoria inativa/deletada;
- atualização de `nota_despertadora` de regra raiz;
- listagem e detalhe de regra raiz contendo `nota_despertadora`;
- rejeição de criação de regra descendente com `nota_despertadora`;
- rejeição de atualização de regra descendente com `nota_despertadora`;
- reativação de regra raiz validando novamente a categoria despertadora.

#### Execução automática

- lançamento de nota na categoria despertadora executa avaliação final raiz aplicável;
- atualização de nota na categoria despertadora recalcula ou mantém idempotência conforme contrato existente;
- lançamento de nota em outra categoria não executa avaliação final automática;
- estudante sem nota na categoria despertadora não é avaliado automaticamente;
- execução automática respeita nível, curso, ano académico, período, matéria e matérias aplicáveis;
- execução automática é idempotente no mesmo contexto;
- regra descendente é acionada por reprovação na raiz, não por nota despertadora;
- nota despertadora não dispara regra dependente diretamente;
- conflito de mais de uma regra raiz aplicável falha ou resolve de forma determinística conforme contrato documentado.

#### Persistência e replay

- evento de regra raiz preserva `nota_despertadora`;
- projeção preserva e expõe `nota_despertadora`;
- replay reconstrói regra raiz com `nota_despertadora`;
- migração mantém regras antigas em estado coerente;
- ausência do campo em regra antiga segue comportamento documentado.

#### Documentação e regressão

- documentação funcional descreve `nota_despertadora` como código da categoria de nota;
- documentação de API mostra exemplos de criação/edição de regra raiz;
- documentação de API mostra erro ao enviar `nota_despertadora` em regra descendente;
- documentação não sugere que descendente possua categoria despertadora própria;
- testes de regressão garantem que categorias fixas antigas ou aliases indevidos não substituem `nota_despertadora`.

## Documentação obrigatória

Atualizar e conferir `docs/Spuri - Documentação.md` e `docs/Spuri - API.md` para explicar:

1. o que é `nota_despertadora`;
2. que o valor é o `codigo` da categoria de nota;
3. que o campo existe apenas em regra raiz;
4. que regras dependentes são disparadas automaticamente por reprovação ancestral;
5. quando a avaliação automática é executada;
6. exemplos de payload de criação e edição de regra raiz;
7. exemplo de erro ao enviar `nota_despertadora` em regra descendente;
8. comportamento de regras antigas sem o campo;
9. relação entre `nota_despertadora`, fórmula, matérias aplicáveis, pendências e regras descendentes;
10. garantias de idempotência e auditoria.

## Critérios de aceite

A tarefa só pode ser considerada concluída quando:

- a auditoria classificar todas as ocorrências relevantes encontradas;
- `nota_despertadora` estiver implementado e documentado no contrato público de regras raízes;
- regras descendentes rejeitarem o campo de forma clara;
- o fluxo de lançamento/atualização de nota disparar automaticamente a avaliação final quando a categoria corresponder ao código configurado;
- a execução automática respeitar escopo, idempotência e auditoria;
- eventos, projeções, respostas e replay preservarem o campo;
- testes cobrirem contrato, execução automática, persistência e regressões;
- `docs/Spuri - Documentação.md` e `docs/Spuri - API.md` estiverem atualizados com exemplos e erros;
- não existir comportamento silencioso em que regra dependente seja despertada diretamente por nota;
- o título desta tarefa tiver recebido o sufixo ` (feito)`;
- este arquivo tiver sido movido para `docs/Tarefas feitas`.
