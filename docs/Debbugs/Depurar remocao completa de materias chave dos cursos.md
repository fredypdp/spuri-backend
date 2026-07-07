---
modificado: 2026-07-07 21:14
criado: 2026-07-07 21:14
---
# Depurar remoção completa de matérias-chave dos cursos

Tarefa: [[Remover completamente materias chave dos cursos]]

## Objetivo da auditoria

Fazer uma auditoria crítica, completa, extremamente profunda e arquivo por arquivo da implementação da tarefa:

`docs/Lista de tarefas/Remover completamente materias chave dos cursos.md`

Esta é uma tarefa de **depuração orientada**, não um relatório de execução. Ao executá-la futuramente, a pessoa ou agente responsável deve investigar o código real, confirmar se a tarefa original foi implementada corretamente e, caso qualquer parte esteja incompleta, inconsistente, parcial, sem teste, sem migration, sem validação, sem documentação ou com compatibilidade silenciosa, deve **terminar a implementação e corrigir o que estiver errado** no mesmo ciclo.

A depuração só pode ser considerada concluída quando estiver comprovado que o conceito de matérias-chave de curso foi removido do domínio ativo. A remoção deve ser real, e não apenas ocultação de UI, omissão em documentação, compatibilidade temporária, alias legado, campo ignorado silenciosamente ou rota mantida sem efeito.

## Regra oficial obrigatória

A implementação final deve obedecer exatamente à decisão de produto abaixo:

- cursos de qualquer nível não aceitam, persistem, retornam nem calculam `materias_chave`;
- não existe rota ativa para configurar matérias-chave de curso;
- avaliação final, progressão, pendências, relatórios, snapshots e auditorias não dependem de matérias-chave;
- contratos públicos e internos não aceitam `materias_chave`, `materiasChave`, `MateriasChave`, `matérias-chave`, `materias-chave`, `materias chave`, `matérias chave` nem aliases equivalentes;
- payloads antigos contendo matérias-chave devem ser rejeitados como contrato inválido, exceto quando o padrão real do endpoint já rejeitar campos desconhecidos automaticamente;
- não pode haver aceitação silenciosa, normalização, conversão, fallback, tolerância ou compatibilidade retroativa para matérias-chave;
- documentação funcional, documentação de API, manuais, exemplos JSON e tarefas atuais devem refletir a nova versão do código sem indicar que matérias-chave ainda fazem parte do modelo.

## Resultado esperado da depuração

Ao executar este debug, ele só poderá ser encerrado quando estiver garantido que:

- a rota específica de matérias-chave foi removida do servidor, dos handlers, dos testes e da documentação;
- DTOs, structs públicas, eventos novos, projections, snapshots e responses ativas não expõem matérias-chave;
- nenhum endpoint de curso aceita matérias-chave em criação, edição, batch, importação ou atualização parcial;
- avaliação final média e regras de progressão funcionam sem buscar matérias-chave no curso;
- não existe branch de regra de negócio que diferencie matéria-chave de matéria não-chave;
- migrations removem persistência ativa associada a matérias-chave, quando houver coluna, índice, constraint ou JSON projetado atual;
- rebuild/replay de projections não reintroduz matérias-chave no modelo público atual;
- testes e fixtures foram atualizados para não montar cursos com matérias-chave;
- a documentação esteja de acordo com a nova versão do código;
- ocorrências restantes do termo estejam restritas a histórico aceitável, migrations antigas ou tarefas feitas, sempre justificadas na entrega;
- a tarefa original receba o sufixo `(feito)` no **título interno do Markdown**, não no nome do arquivo, e seja movida de `docs/Lista de tarefas/` para `docs/Tarefas feitas/` somente depois de tudo estar implementado, testado e documentado.

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. registro de rotas HTTP, middlewares e agrupamentos de rotas da academia;
2. handlers de criação, edição, leitura, listagem, batch, importação e configuração de cursos;
3. handlers, services, aggregates e projections de avaliação final;
4. regras de progressão, pendências, conclusão, aprovação, reprovação e relatórios acadêmicos;
5. DTOs públicos, DTOs internos, structs de request/response, serializers e validações customizadas;
6. aggregates, comandos, eventos novos, aplicadores de eventos, snapshots e reconstrução de estado de cursos;
7. projections, repositories, queries SQL, JSONB e modelos de leitura de cursos;
8. migrations, seeds, scripts operacionais e rotinas de rebuild/replay;
9. testes unitários, testes de aggregate, testes de handler, testes de projection, fixtures, builders e helpers;
10. documentação funcional, documentação de API, manual de configuração inicial da academia e exemplos JSON;
11. tarefas feitas ou documentos históricos que mencionem matérias-chave e possam confundir documentação atual;
12. comportamento para payloads legados contendo matérias-chave;
13. contratos públicos gerados, OpenAPI ou documentação equivalente, se existirem;
14. logs, mensagens de erro e auditorias que mencionem configuração ausente de matérias-chave.

## Checklist obrigatório de validação

### 1. Rotas e superfície HTTP

Auditar o servidor e confirmar que não existe rota ativa para configurar matérias-chave.

Validar que:

- `PUT /academia/curso/:id/materias-chave`, `PUT /academia/cursos/:id/materias-chave` ou qualquer rota equivalente foi removida;
- não existe handler ativo com responsabilidade de configurar, substituir, limpar ou validar matérias-chave;
- o registro da rota foi removido de todos os grupos versionados e não versionados;
- documentação de rotas, exemplos e testes não instruem o cliente a chamar endpoint de matérias-chave;
- chamadas para a rota antiga retornam comportamento natural de rota inexistente, e não sucesso sem efeito;
- não existe endpoint administrativo, batch, importação ou manutenção que preserve a mesma capacidade com outro caminho.

### 2. Criação, edição, leitura e listagem de cursos

Auditar todos os fluxos que criam, atualizam, consultam ou listam cursos.

Validar que:

- requests de criação de curso não possuem campo de matérias-chave;
- requests de edição total ou parcial de curso não possuem campo de matérias-chave;
- payloads com `materias_chave`, `materiasChave` ou `MateriasChave` são rejeitados, ou caem na rejeição padrão de campos desconhecidos;
- não existe aceitação silenciosa de campos antigos;
- responses de criação, leitura e listagem de cursos não expõem matérias-chave;
- projection e modelo de leitura usados pelas responses não mantêm o campo ativo;
- curso médio, fundamental, superior ou qualquer outro tipo segue a mesma regra de remoção total;
- não há reintrodução do conceito por nomes alternativos como `materias_principais`, `disciplinas_chave`, `disciplinas_obrigatorias` ou equivalentes que preservem a mesma semântica.

### 3. DTOs, serializers e validação de contrato

Auditar todos os contratos públicos e internos.

Validar que foram removidos:

- campos `materias_chave`, `materiasChave`, `MateriasChave` e equivalentes;
- tipos ou structs de configuração de matérias-chave por ano acadêmico;
- aliases JSON antigos;
- validadores específicos de matérias-chave;
- mensagens de erro que orientem configurar matérias-chave;
- helpers de normalização, parsing, deduplicação ou comparação de matérias-chave;
- exemplos de payload contendo matérias-chave.

Quando o endpoint aceita campos extras por padrão, avaliar e corrigir os endpoints afetados para rejeitar explicitamente campos de matérias-chave, evitando compatibilidade silenciosa.

### 4. Domínio, comandos, eventos e snapshots

Auditar aggregates e modelos de domínio de curso e avaliação final.

Validar que:

- comandos novos de criação/edição/configuração de curso não carregam matérias-chave;
- eventos novos não possuem payload de matérias-chave;
- aplicadores de eventos não reconstroem matérias-chave no estado ativo;
- snapshots atuais de curso não expõem nem dependem de matérias-chave;
- eventos históricos permanecem apenas como histórico bruto, sem virar contrato público atual;
- reconstrução/replay não reintroduz campo removido na projection ativa;
- não existe método como `ConfigurarMateriasChave`, `MateriasChavePorAno`, `IsMateriaChave` ou equivalente em uso ativo;
- invariantes de curso não exigem configuração de matérias-chave.

### 5. Persistência, migrations e rebuild de projections

Auditar schema, migrations e projections.

Validar que:

- colunas ativas como `materias_chave` foram removidas quando existirem;
- índices, constraints, defaults, JSON schemas ou views associados foram removidos;
- migrations novas são idempotentes quando esse for o padrão do projeto;
- migrations históricas já aplicadas podem permanecer como registro, mas não justificam suporte ativo;
- queries SQL não selecionam, inserem, atualizam ou filtram por matérias-chave;
- rebuild de projection a partir de eventos antigos não publica matérias-chave no modelo atual;
- seeds e scripts não criam cursos com matérias-chave;
- não há transformação dos dados antigos em campo substituto equivalente.

### 6. Avaliação final, progressão e pendências

Auditar profundamente a regra acadêmica dependente de cursos e matérias.

Validar que:

- avaliação final do médio não busca matérias-chave no curso;
- aprovação, reprovação, pendências e progressão não diferenciam matéria-chave de matéria não-chave;
- regras descendentes, fórmulas, nota mínima e limites de pendência usam apenas os conceitos vigentes;
- snapshots/auditoria da avaliação final não registram lista de matérias-chave usada;
- logs não dizem que uma configuração de matérias-chave foi encontrada, ausente ou inválida;
- erros antigos como “curso sem matérias-chave configuradas” foram removidos;
- testes de avaliação final demonstram funcionamento sem matérias-chave;
- nenhum critério substituto recria o conceito com outro nome.

### 7. Testes, fixtures, builders e mocks

Auditar e corrigir todos os testes impactados.

Validar que:

- não há fixture que crie curso médio com matérias-chave por padrão;
- builders de curso não possuem campo ou método de matérias-chave;
- testes de curso cobrem criação e edição sem matérias-chave;
- testes de contrato cobrem rejeição de payloads com matérias-chave quando o endpoint não rejeitar campos desconhecidos automaticamente;
- testes confirmam inexistência da rota específica;
- testes de response confirmam ausência do campo em cursos;
- testes de avaliação final média não dependem de matérias-chave;
- snapshots esperados foram atualizados para remover listas de matérias-chave;
- testes antigos que validavam configuração de matérias-chave foram removidos ou reescritos para validar a ausência do conceito.

### 8. Busca ampla obrigatória

Fazer busca ampla e classificar cada ocorrência encontrada como válida, histórica/documental aceitável ou bug ativo.

Pesquisar, no mínimo:

```bash
rg -n "materias_chave|materiasChave|MateriasChave|matérias-chave|materias-chave|materias chave|matérias chave|chave" .
```

Também pesquisar variações que possam esconder a mesma regra:

```bash
rg -n "materia.*chave|disciplina.*chave|principal|obrigatoria|obrigatória|ConfigurarMaterias|IsMateria|materias_principais|disciplinas_chave" .
```

Para cada ocorrência relacionada ao conceito removido, classificar como uma das opções:

- código ativo a remover;
- teste/fixture a atualizar;
- documentação atual a corrigir;
- migration histórica aceitável;
- tarefa histórica em `docs/Tarefas feitas` aceitável apenas como registro do passado;
- falso positivo sem relação com matérias-chave de curso.

Não basta listar ocorrências. Cada ocorrência relevante deve ser analisada no contexto do arquivo, do fluxo de execução e do contrato público.

### 9. Documentação obrigatória

Confirmar e corrigir, se necessário, que toda documentação esteja de acordo com a nova versão do código.

Auditar no mínimo:

- `docs/Manual de Configuração Inicial da Academia.md`;
- documentação de cursos;
- documentação de matérias disciplinares;
- documentação de avaliação final;
- documentação de progressão, pendências e conclusão;
- exemplos de payload de criação e edição de curso;
- exemplos de resposta de curso;
- fluxos de configuração inicial de academia;
- orientações operacionais que antes mandavam configurar matérias-chave;
- tarefas atuais em `docs/Lista de tarefas/`;
- documentos históricos em `docs/Tarefas feitas/`, apenas para garantir que não estejam sendo usados como documentação vigente.

A documentação atual deve deixar explícito que:

- matérias-chave de curso não fazem parte do modelo atual;
- não existe etapa de configuração de matérias-chave;
- cursos são criados e editados sem matérias-chave;
- matérias disciplinares continuam existindo como disciplinas do curso/ano quando aplicável, mas sem classificação como chave;
- avaliação final e progressão não dependem de uma lista de matérias-chave;
- exemplos JSON não incluem `materias_chave`, `materiasChave` ou aliases;
- o fluxo inicial correto é configurar anos letivos/anos acadêmicos, criar cursos, criar matérias disciplinares, configurar categorias/regras de avaliação final e então operar matrículas, notas e faltas.

### 10. Correções esperadas quando houver divergência

Se a auditoria encontrar qualquer divergência, implementar a correção no mesmo ciclo de depuração. Exemplos de correções esperadas:

1. remover rota e handler de configuração de matérias-chave;
2. remover campos dos DTOs de curso e avaliação final;
3. rejeitar explicitamente payloads legados quando o decoder não rejeitar campos desconhecidos;
4. remover métodos, helpers e validações de matérias-chave;
5. ajustar aggregates, eventos novos e projections para não carregarem o conceito;
6. criar migration para remover coluna, índice, constraint ou estrutura persistente ativa;
7. ajustar replay/rebuild para não publicar matérias-chave no modelo atual;
8. remover branch de avaliação final/progressão baseada em matérias-chave;
9. atualizar fixtures, builders e testes;
10. atualizar documentação e exemplos;
11. mover a tarefa original para feitas somente após código, testes e documentação estarem concluídos.

## Comandos mínimos de validação esperados

Ao executar este debug futuramente, rodar no mínimo:

```bash
rg -n "materias_chave|materiasChave|MateriasChave|matérias-chave|materias-chave|materias chave|matérias chave" .
rg -n "materia.*chave|disciplina.*chave|materias_principais|disciplinas_chave|ConfigurarMaterias|IsMateria" .
go test ./...
```

Se algum comando não puder ser executado por limitação de ambiente, registrar a limitação explicitamente na entrega.

## Critério de aceite final

A depuração só pode ser considerada concluída quando:

- todos os itens do checklist forem verificados;
- cada bug encontrado for corrigido no código, nas migrations, nos testes e/ou na documentação;
- a suíte relevante de testes passar;
- a documentação estiver sincronizada com o comportamento real do backend;
- não existir rota ativa para configurar matérias-chave;
- nenhum request público aceitar `materias_chave`, `materiasChave`, `MateriasChave` ou equivalente;
- nenhum response público de curso, regra ou avaliação final expuser matérias-chave;
- não existir código ativo que diferencie matéria-chave de matéria não-chave;
- avaliação final média funcionar sem buscar matérias-chave no curso;
- migrations removerem persistência ativa de matérias-chave quando aplicável;
- testes e fixtures não dependerem de matérias-chave;
- `docs/Manual de Configuração Inicial da Academia.md` estiver atualizado;
- documentação atual não instruir configurar matérias-chave;
- ocorrências restantes do termo estiverem restritas a histórico aceitável ou falsos positivos justificados;
- a tarefa original tiver o título interno alterado para `# Remover completamente o conceito de matérias-chave dos cursos (feito)`;
- o arquivo da tarefa original for movido de `docs/Lista de tarefas/` para `docs/Tarefas feitas/`, mantendo o nome do arquivo sem adicionar `(feito)`.

## Entrega esperada da execução futura

Quando este debug for executado, a entrega final deve informar:

- arquivos e camadas auditados;
- ocorrências encontradas e classificação de cada uma;
- bugs ou lacunas encontrados;
- correções aplicadas;
- migrations criadas ou justificativa para não criar migration;
- testes criados, removidos ou atualizados;
- documentação atualizada;
- comandos executados e resultados;
- confirmação explícita de que a documentação está de acordo com a nova versão do código.
