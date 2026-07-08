---
modificado: 2026-07-08 00:00
criado: 2026-07-08 00:00
---
# Depurar implementação de pendências exclusivas do ensino superior e matérias superiores ativadas por padrão

Tarefa: [[Restringir pendências ao ensino superior e ativar matérias superiores por padrão]]

## Objetivo da auditoria

Fazer uma auditoria crítica, completa, extremamente profunda e arquivo por arquivo da implementação da tarefa:

`docs/Lista de tarefas/Restringir pendencias ao ensino superior e ativar materias superiores por padrao.md`

Esta é uma tarefa de **depuração orientada**, não um relatório de execução. Ao executá-la futuramente, a pessoa ou agente responsável deve investigar o código real, confirmar se a tarefa original foi implementada corretamente e, caso qualquer parte esteja incompleta, inconsistente, parcial, sem teste, sem migration, sem validação, sem documentação, com contrato ambíguo ou com compatibilidade silenciosa indevida, deve **terminar a implementação e corrigir o que estiver errado** no mesmo ciclo.

A depuração só pode ser considerada concluída quando estiver comprovado que pendência acadêmica é um recurso funcionalmente exclusivo de matérias do ensino superior, que matérias escolares não aceitam nem expõem configuração de pendência, que matérias superiores novas nascem com pendência permitida e status `ativada` por padrão, e que toda documentação vigente está coerente com a nova versão do código.

## Regra oficial obrigatória

A implementação final deve obedecer exatamente à decisão de produto abaixo:

- `pendencia_permitida` é campo exclusivo de matérias do ensino superior;
- `pendencia_nivel_conclusao` é campo exclusivo de matérias do ensino superior;
- matérias escolares não aceitam, persistem, retornam nem usam campos de pendência;
- matéria superior criada sem `pendencia_permitida` explícito deve ser persistida e retornada com `pendencia_permitida = true`;
- matéria superior criada sem `status` explícito deve ser persistida e retornada com status `ativada`;
- valores explícitos válidos enviados para matérias superiores devem respeitar o contrato vigente, sem serem sobrescritos silenciosamente;
- qualquer tentativa de configurar pendência em matéria escolar deve falhar de forma clara ou ser impossível pelo contrato específico do endpoint;
- não pode haver alias, wrapper, fallback, flag, modo legado, normalização silenciosa, bypass administrativo ou compatibilidade temporária que permita pendência em matéria escolar;
- documentação funcional, documentação técnica, OpenAPI/Swagger, exemplos JSON, coleções e tarefas atuais devem refletir a nova versão do código.

## Resultado esperado da depuração

Ao executar este debug, ele só poderá ser encerrado quando estiver garantido que:

- todos os fluxos de criação, edição, importação, seed, administração e manutenção de matérias escolares rejeitam ou impedem configuração de pendência;
- todos os fluxos de criação de matérias superiores aplicam `pendencia_permitida = true` quando o campo não é enviado;
- todos os fluxos de criação de matérias superiores aplicam status `ativada` quando o status não é enviado;
- respostas de matérias escolares não expõem pendência como recurso configurável;
- respostas de matérias superiores expõem os valores efetivos de pendência e status conforme o contrato vigente;
- domínio, commands, eventos, snapshots, projections, repositories, queries, migrations e serializers não tratam pendência escolar como estado válido;
- regras de progressão, pendência, conclusão, avaliação final, relatórios e auditorias usam pendência apenas para ensino superior;
- testes automatizados cobrem exclusividade escolar, padrão de pendência superior, status padrão superior, respostas e regressões relevantes;
- OpenAPI/Swagger e documentação textual diferenciam claramente contratos escolares e superiores;
- ocorrências restantes dos termos de pendência em contexto escolar estejam restritas a histórico aceitável ou documentação de tarefas antigas, sempre justificadas na entrega;
- a tarefa original receba o sufixo `(feito)` no **título interno do Markdown**, não no nome do arquivo, e seja movida de `docs/Lista de tarefas/` para `docs/Tarefas feitas/` somente depois de tudo estar implementado, testado e documentado.

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. registro de rotas HTTP, middlewares e agrupamentos de rotas de matérias escolares e superiores;
2. handlers de criação, edição, leitura, listagem, batch, importação e configuração de matérias;
3. DTOs públicos, DTOs internos, schemas, serializers, validadores e normalizadores de matérias;
4. commands, casos de uso, services, aggregates, eventos, aplicadores e snapshots de matérias;
5. projections, repositories, queries SQL, modelos de leitura e rebuild/replay de matérias;
6. migrations, constraints, índices, defaults, seeds, factories e scripts operacionais;
7. regras de progressão superior, pendências, conclusão, reprovação, avaliação final, relatórios e auditorias;
8. fluxos escolares de fundamental e médio que possam ter herdado campos de pendência por tabela ou DTO compartilhado;
9. testes unitários, testes de handler, testes de domínio, testes de projection, testes de migration, fixtures, builders, mocks e snapshots;
10. documentação funcional, documentação técnica, OpenAPI/Swagger, exemplos JSON, coleções de API, README e guias operacionais;
11. tarefas atuais em `docs/Lista de tarefas/` e documentos em `docs/Tarefas feitas/` que possam estar sendo usados como referência ativa;
12. comportamento para payloads legados contendo `pendencia_permitida` ou `pendencia_nivel_conclusao` em matérias escolares;
13. contratos gerados, clientes tipados, SDKs internos ou documentação derivada, se existirem;
14. logs, mensagens de erro e auditorias que mencionem pendência escolar ou configuração ausente de pendência em matéria escolar.

## Checklist obrigatório de validação

### 1. Busca ampla e classificação de ocorrências

Fazer busca ampla no repositório e classificar cada ocorrência como válida, histórica/documental aceitável ou bug ativo.

Pesquisar, no mínimo:

```bash
rg -n "pendencia_permitida|pendenciaPermitida|PendenciaPermitida|pendência permitida|pendencia permitida|pendência_permitida" .
rg -n "pendencia_nivel_conclusao|pendenciaNivelConclusao|PendenciaNivelConclusao|nivel_conclusao|nível conclusão|nivel conclusao" .
rg -n "pendencia|pendência|dependencia|dependência|materia superior|matéria superior|materia escolar|matéria escolar" .
rg -n "ativada|ativo|inativa|status" internal docs migrations .
```

Para cada ocorrência relacionada ao tema, classificar como uma das opções:

- código ativo correto para matéria superior;
- código ativo incorreto a corrigir;
- contrato escolar indevido a remover;
- teste/fixture/mock a atualizar;
- documentação vigente a corrigir;
- OpenAPI/Swagger ou contrato gerado a atualizar;
- migration histórica aceitável;
- tarefa histórica aceitável apenas como registro do passado;
- falso positivo sem relação com pendência acadêmica de matéria.

Não basta listar ocorrências. Cada ocorrência relevante deve ser analisada no contexto do arquivo, do fluxo de execução, do contrato público e da persistência.

### 2. Rotas e superfície HTTP

Auditar todas as rotas de matérias para identificar se há contratos separados ou compartilhados entre escolar e superior.

Validar que:

- endpoints de criação de matéria escolar não aceitam `pendencia_permitida` nem `pendencia_nivel_conclusao`;
- endpoints de atualização total ou parcial de matéria escolar não aceitam campos de pendência;
- endpoints de importação, batch, seed operacional ou administração não permitem configurar pendência escolar;
- endpoints de matéria superior aceitam campos de pendência apenas dentro das regras superiores;
- criação de matéria superior sem `pendencia_permitida` retorna o valor efetivo `true`;
- criação de matéria superior sem `status` retorna o status efetivo `ativada`;
- erros de contrato são claros, determinísticos e auditáveis;
- não existe rota alternativa, parâmetro oculto, query string, header, endpoint administrativo ou feature flag que reative pendência em matéria escolar.

### 3. Criação de matérias escolares

Auditar os fluxos de criação de matérias de ensino fundamental, médio e qualquer outra categoria escolar.

Validar que:

- payload escolar com `pendencia_permitida` é rejeitado ou impossível pelo DTO específico;
- payload escolar com `pendencia_nivel_conclusao` é rejeitado ou impossível pelo DTO específico;
- payload escolar com ambos os campos é rejeitado de forma explícita;
- campos com alias ou variação de casing não são aceitos silenciosamente;
- matéria escolar criada sem campos de pendência não recebe pendência funcional por default;
- se a tabela física for compartilhada, valores nulos, falsos ou defaults técnicos não viram contrato público escolar;
- responses escolares não instruem o cliente a configurar pendência;
- logs, eventos e auditorias de criação escolar não registram pendência como decisão de negócio.

### 4. Atualização de matérias escolares

Auditar todos os fluxos que alteram matérias escolares existentes.

Validar que:

- atualização total não aceita inserir ou alterar campos de pendência;
- atualização parcial não aceita inserir ou alterar campos de pendência;
- importações e sincronizações não conseguem criar pendência funcional em matéria escolar;
- factories e scripts administrativos não usam pendência escolar como atalho;
- payloads com campos desconhecidos não são ignorados silenciosamente quando isso puder esconder erro de cliente;
- matérias escolares antigas que eventualmente possuam valores residuais não expõem nem usam esses valores;
- se houver necessidade de limpeza de dados, a migration ou script deve ser explícito, seguro e testável.

### 5. Criação de matérias superiores

Auditar o fluxo completo de criação de matérias superiores.

Validar que:

- matéria superior criada sem `pendencia_permitida` recebe `pendencia_permitida = true` antes de persistir;
- matéria superior criada com `pendencia_permitida = true` mantém `true`;
- matéria superior criada com `pendencia_permitida = false`, se o contrato permitir desativação explícita, mantém `false` e documenta a decisão;
- matéria superior criada sem `status` recebe status `ativada` antes de persistir;
- matéria superior criada com status `ativada` mantém `ativada`;
- matéria superior criada com status inválido é rejeitada;
- matéria superior criada com `pendencia_nivel_conclusao` válido persiste e retorna o valor;
- matéria superior criada com `pendencia_nivel_conclusao` inválido é rejeitada;
- os defaults são aplicados em camada única e previsível, sem duplicações contraditórias entre handler, service, aggregate e database default;
- eventos, projections e responses carregam os valores efetivos aplicados.

### 6. Atualização de matérias superiores

Auditar os fluxos de atualização de matérias superiores.

Validar que:

- atualização de `pendencia_permitida` em matéria superior segue as regras permitidas pelo produto;
- atualização de `pendencia_nivel_conclusao` em matéria superior valida valores e dependências corretamente;
- atualização de status em matéria superior valida somente status aceitos pelo domínio;
- atualização parcial diferencia campo ausente de campo enviado com `false`, `null` ou valor inválido;
- defaults de criação não sobrescrevem valores explícitos em edição;
- projections e responses refletem exatamente o estado persistido;
- testes cobrem ausência, `true`, `false`, `null`, valor inválido e campos desconhecidos.

### 7. DTOs, serializers e validação de contrato

Auditar todos os contratos públicos e internos.

Validar que:

- existe separação clara entre DTO escolar e DTO superior, ou validação condicional rigorosa por nível;
- DTO escolar não documenta nem aceita `pendencia_permitida` e `pendencia_nivel_conclusao`;
- DTO superior documenta os campos de pendência e seus defaults;
- serializers de resposta escolar não expõem pendência como configuração válida;
- serializers de resposta superior expõem valores efetivos conforme contrato;
- validação de campos desconhecidos não permite tolerância silenciosa em payloads escolares;
- aliases como `pendenciaPermitida`, `PendenciaPermitida`, `pendencia_nivel`, `nivelConclusaoPendencia` ou equivalentes não são aceitos fora do contrato;
- mensagens de erro não falam em pendência escolar como recurso configurável;
- exemplos JSON de request e response foram atualizados.

### 8. Domínio, commands, eventos e snapshots

Auditar aggregates e modelos de domínio de matérias.

Validar que:

- commands escolares não carregam campos de pendência;
- commands superiores aplicam ou recebem defaults de forma explícita;
- eventos novos de matéria escolar não possuem payload funcional de pendência;
- eventos novos de matéria superior registram os valores efetivos de pendência e status;
- aplicadores de eventos não reconstroem pendência escolar como estado ativo;
- snapshots atuais não expõem pendência escolar como contrato;
- replay de eventos antigos não transforma pendência escolar residual em comportamento vigente;
- invariantes de matéria escolar proíbem pendência funcional;
- invariantes de matéria superior validam defaults, status e nível de conclusão de pendência.

### 9. Persistência, migrations e projections

Auditar schema, migrations, repositories e projections.

Validar que:

- se campos de pendência estiverem em tabela compartilhada, constraints ou validações impedem uso funcional para matéria escolar;
- se houver projections separadas, a projection escolar não publica campos de pendência;
- queries escolares não selecionam, inserem, atualizam, filtram nem ordenam por pendência como regra de negócio;
- queries superiores persistem e retornam os valores efetivos;
- defaults de banco não entram em conflito com defaults do domínio;
- migrations novas são idempotentes quando esse for o padrão do projeto;
- dados legados escolares com pendência são limpos, neutralizados ou ignorados com justificativa explícita e segura;
- rebuild/replay de projections não reintroduz pendência escolar;
- seeds, fixtures e scripts não criam matérias escolares com pendência funcional;
- não há transformação de pendência escolar antiga em outro campo equivalente.

### 10. Regras acadêmicas de progressão, pendência e conclusão

Auditar todos os pontos que usam pendência para decisão acadêmica.

Validar que:

- pendência só influencia progressão no ensino superior;
- conclusão escolar não consulta `pendencia_permitida` nem `pendencia_nivel_conclusao`;
- avaliação final escolar não usa pendência como exceção de aprovação, reprovação ou progressão;
- relatórios escolares não mostram matéria escolar como pendente por configuração de matéria;
- relatórios superiores usam o valor efetivo da matéria superior;
- auditoria de progressão superior registra claramente quando uma pendência foi permitida;
- auditoria escolar não registra pendência como decisão aplicável;
- testes cobrem casos em que uma matéria escolar possui resíduo persistido e mesmo assim a regra escolar não o utiliza.

### 11. Status padrão `ativada` para matéria superior

Auditar a regra de status especificamente.

Validar que:

- o default `ativada` é aplicado apenas na criação de matéria superior;
- o default não altera indevidamente o comportamento vigente de matérias escolares;
- status omitido e status explicitamente vazio/nulo são tratados conforme contrato definido;
- matéria superior recém-criada aparece em listagens e fluxos que dependem de matéria ativa;
- matéria superior com status explícito permitido diferente de `ativada`, se existir, preserva o valor explícito;
- status inválido falha antes de persistir;
- docs e OpenAPI descrevem o default `ativada` de forma inequívoca.

### 12. Testes, fixtures, builders e mocks

Auditar e corrigir todos os testes impactados.

Validar que existam testes para:

- criação escolar com `pendencia_permitida` rejeitada;
- criação escolar com `pendencia_nivel_conclusao` rejeitada;
- atualização escolar com campos de pendência rejeitada;
- importação ou batch escolar com campos de pendência rejeitado;
- response escolar sem pendência como configuração válida;
- criação superior sem `pendencia_permitida` aplicando `true`;
- criação superior com `pendencia_permitida = true` mantendo `true`;
- criação superior com `pendencia_permitida = false`, se permitido, mantendo `false`;
- criação superior sem status aplicando `ativada`;
- criação superior com status explícito válido preservado;
- criação superior com status inválido rejeitada;
- `pendencia_nivel_conclusao` válido e inválido em matéria superior;
- progressão superior usando o default efetivo de pendência;
- progressão escolar ignorando qualquer resíduo de pendência;
- rebuild/replay de projection sem reintroduzir pendência escolar;
- OpenAPI/Swagger ou snapshots de contrato atualizados.

Também validar que fixtures e builders:

- não criam matéria escolar com pendência por padrão;
- criam matéria superior com defaults coerentes;
- não escondem o comportamento real usando helpers que preenchem valores diferentes do contrato;
- têm nomes claros para casos escolares e superiores.

### 13. OpenAPI, Swagger e contratos gerados

Auditar documentação gerada e contratos públicos.

Validar que:

- schema de matéria escolar não possui `pendencia_permitida` nem `pendencia_nivel_conclusao`;
- schema de matéria superior possui os campos de pendência com descrição de exclusividade;
- schema de criação de matéria superior documenta default `pendencia_permitida = true`;
- schema de criação de matéria superior documenta default de status `ativada`;
- exemplos de request escolar não contêm campos de pendência;
- exemplos de response escolar não sugerem pendência configurável;
- exemplos de request/response superior mostram os valores efetivos esperados;
- clientes ou contratos gerados foram atualizados, se o projeto os versionar;
- documentação não chama os campos de pendência escolar de deprecated, pois eles não fazem parte do contrato escolar vigente.

### 14. Documentação obrigatória

Confirmar e corrigir, se necessário, que toda documentação esteja de acordo com a nova versão do código.

Auditar no mínimo:

- `docs/Manual de Configuração Inicial da Academia.md`;
- documentação de matérias escolares;
- documentação de matérias superiores;
- documentação de cursos;
- documentação de progressão acadêmica;
- documentação de pendências e conclusão;
- documentação de avaliação final;
- documentação de API/OpenAPI/Swagger;
- exemplos JSON de criação, edição, leitura e listagem de matérias;
- guias operacionais e coleções de API;
- tarefas atuais em `docs/Lista de tarefas/`;
- documentos históricos em `docs/Tarefas feitas/`, apenas para garantir que não estejam sendo usados como documentação vigente.

A documentação atual deve deixar explícito que:

- `pendencia_permitida` e `pendencia_nivel_conclusao` são exclusivos de matérias do ensino superior;
- matérias escolares não aceitam nem expõem configuração de pendência;
- matéria superior criada sem `pendencia_permitida` assume `true`;
- matéria superior criada sem status assume `ativada`;
- não existe modo legado para configurar pendência em matéria escolar;
- exemplos e descrições refletem exatamente o comportamento implementado no código.

### 15. Dados legados e migração

Investigar se há dados existentes de matéria escolar com pendência configurada.

Validar que:

- há diagnóstico explícito para dados legados, caso o ambiente possua esses campos;
- qualquer limpeza de dados é feita por migration, script controlado ou estratégia documentada;
- a implementação não depende apenas de “não usar na UI”;
- constraints ou validações impedem recriação do estado inválido;
- replay/rebuild não ressuscita dados escolares inválidos;
- a entrega documenta o que foi feito com dados antigos e por quê.

### 16. Compatibilidade proibida

Confirmar que a implementação não criou atalhos proibidos.

É obrigatório remover ou corrigir:

- aliases de campos de pendência escolar;
- fallback que trata matéria escolar como superior para aceitar pendência;
- wrappers de compatibilidade em handlers, services ou DTOs;
- feature flags para habilitar pendência escolar;
- tolerância silenciosa a campos de pendência escolares;
- comentários ou TODOs que prometam corrigir depois;
- código morto que ainda aceite pendência escolar;
- documentação que diga que o campo é temporariamente aceito para escola.

## Roteiro sugerido de execução

1. Ler integralmente a tarefa original e registrar os critérios de aceite.
2. Mapear todos os arquivos de matérias, pendências, progressão, status, documentação e contratos gerados.
3. Fazer as buscas amplas obrigatórias e classificar cada ocorrência relevante.
4. Auditar primeiro os contratos HTTP e DTOs, porque eles definem a superfície pública.
5. Auditar domínio, commands, eventos e snapshots para garantir que o estado inválido não entra no sistema.
6. Auditar persistência, projections e migrations para garantir que o estado inválido não permanece funcional.
7. Auditar regras acadêmicas para garantir que escola não usa pendência e superior usa os defaults efetivos.
8. Auditar testes e criar regressões para todos os comportamentos obrigatórios.
9. Auditar documentação textual e gerada, corrigindo qualquer divergência com o código.
10. Executar a suíte de testes relevante e, se possível, a suíte completa.
11. Atualizar a tarefa original para `(feito)` e mover para `docs/Tarefas feitas/` apenas se tudo estiver implementado, testado e documentado.
12. Na entrega, listar ocorrências remanescentes justificadas, testes executados e documentos atualizados.

## Comandos mínimos recomendados

Adaptar os comandos ao stack real do projeto, mas não concluir sem equivalentes a:

```bash
rg -n "pendencia_permitida|pendenciaPermitida|PendenciaPermitida|pendencia_nivel_conclusao|pendenciaNivelConclusao|PendenciaNivelConclusao" .
rg -n "pendencia|pendência|dependencia|dependência" internal docs migrations .
rg -n "ativada|status" internal docs migrations .
rg -n "materia|matéria|disciplina" internal docs migrations .
```

Também executar os testes específicos de matérias, pendências, progressão, projections, handlers e documentação/contrato gerado existentes no projeto. Se algum comando não puder ser executado por limitação de ambiente, registrar a limitação e não tratar como validação funcional concluída.

## Evidências exigidas na entrega

A entrega da execução futura deste debug deve conter:

- arquivos de código corrigidos;
- arquivos de teste criados ou atualizados;
- migrations ou scripts de dados, se necessários;
- documentação atualizada e coerente com o código;
- OpenAPI/Swagger ou contratos gerados atualizados, se existirem;
- resultado dos testes executados;
- lista de ocorrências remanescentes dos termos de pendência e justificativa de cada grupo;
- explicação de como a implementação impede pendência escolar em criação, atualização, importação e regras acadêmicas;
- explicação de onde são aplicados os defaults superiores de `pendencia_permitida = true` e status `ativada`;
- confirmação de que a tarefa original foi marcada como feita e movida somente após validação completa.

## Critérios de bloqueio

Não concluir este debug se qualquer uma das situações abaixo existir:

- matéria escolar ainda aceitar `pendencia_permitida` em qualquer fluxo ativo;
- matéria escolar ainda aceitar `pendencia_nivel_conclusao` em qualquer fluxo ativo;
- response escolar ainda expuser pendência como configuração válida;
- matéria superior criada sem `pendencia_permitida` não retornar `true`;
- matéria superior criada sem status não retornar `ativada`;
- progressão escolar usar pendência para decidir aprovação, reprovação ou conclusão;
- progressão superior ignorar o valor efetivo de pendência;
- OpenAPI/Swagger divergir do comportamento real;
- documentação vigente disser que pendência escolar é permitida, depreciada ou temporariamente aceita;
- testes obrigatórios não existirem ou não cobrirem regressões críticas;
- houver compatibilidade silenciosa, alias, fallback, wrapper, TODO ou código morto mantendo suporte escolar;
- a tarefa original for movida para `docs/Tarefas feitas/` antes de código, testes e documentação estarem coerentes.
