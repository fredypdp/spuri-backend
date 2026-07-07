---
modificado: 2026-07-07 21:14
criado: 2026-07-07 21:14
---
# Depurar implementação de períodos fixos e imutáveis dos anos letivos

Tarefa: [[Implementar periodos fixos e imutaveis dos anos letivos]]

## Objetivo da auditoria

Fazer uma auditoria crítica, completa e arquivo por arquivo da implementação da tarefa:

`docs/Lista de tarefas/Implementar periodos fixos e imutaveis dos anos letivos.md`

A auditoria deve confirmar se a implementação foi feita corretamente, completamente e **à risca**. Caso qualquer parte esteja incompleta, inconsistente, parcialmente implementada, sem validação, sem teste, sem documentação, com comportamento silencioso incorreto ou divergente do contrato esperado, esta tarefa exige **terminar a implementação e corrigir o que estiver errado**.

Esta funcionalidade é crítica porque o período dos anos letivos deixa de ser dado configurável e passa a ser uma regra sistêmica, fixa e imutável, derivada exclusivamente do tipo de ano letivo. Nenhum fluxo pode permitir que academia, admin, payload de API, migração operacional, importação, replay de eventos ou manutenção manual transforme um ano letivo escolar em período superior, ou um ano letivo superior em período escolar.

## Regra oficial obrigatória

A implementação final deve obedecer exatamente à regra abaixo:

| Tipo de ano letivo | Período fixo obrigatório | Janela para `2025_2026` |
| --- | --- | --- |
| `escolar` | `09_07` | `2025-09-01` até `2026-07-31` |
| `superior` | `10_07` | `2025-10-01` até `2026-07-31` |

O identificador do ano letivo continua evolutivo, como `2025_2026`, `2026_2027`, etc. O campo `periodo`, porém, **não pode ser configurável**: ele deve ser sempre resolvido pelo backend a partir do tipo (`escolar` ou `superior`).

## Resultado esperado da depuração

A depuração só pode ser encerrada quando estiver garantido que:

- existe uma regra centralizada, sem strings mágicas espalhadas, para resolver `escolar -> 09_07` e `superior -> 10_07`;
- tipos desconhecidos falham de forma explícita quando a função/regra central não consegue resolver o período;
- criação de ano letivo escolar sempre grava e retorna `09_07`;
- criação de ano letivo superior sempre grava e retorna `10_07`;
- atualizações não permitem alterar apenas `periodo`, nem gravar período incompatível com o tipo;
- nenhum handler, service, aggregate, projection, migration, seed, job ou fluxo assíncrono usa `periodo` vindo do cliente como fonte de verdade;
- validações de faltas, matrículas, vínculos, progressão e encerramento usam a janela letiva derivada do tipo e do identificador do ano letivo;
- dados legados incompatíveis são corrigidos por migração segura ou bloqueados com erro claro e auditável;
- a documentação funcional, documentação de API e manuais estejam de acordo com a nova versão do código;
- a tarefa original receba o sufixo `(feito)` no **título interno do Markdown**, não no nome do arquivo, e seja movida de `docs/Lista de tarefas/` para `docs/Tarefas feitas/` somente depois de tudo estar implementado, testado e documentado.

## Escopo mínimo da investigação

Antes de concluir a auditoria, investigar no mínimo:

1. modelos de domínio relacionados a anos letivos, tipo de ensino, período, identificador e janelas de data;
2. aggregates, eventos e snapshots que criam, atualizam, finalizam ou reconstroem anos letivos;
3. handlers e DTOs de criação, edição, leitura, listagem, finalização e qualquer fluxo administrativo de anos letivos;
4. services, use cases e validações que calculam período letivo real;
5. projections de anos letivos, academias, estudantes, faltas, turmas, cursos e avaliação final que leem ou persistem `periodo`;
6. migrations que criam, alteram, normalizam ou fazem backfill de anos letivos e configurações acadêmicas;
7. seeds, scripts operacionais, jobs assíncronos e rotinas de manutenção que possam inserir ou atualizar `periodo`;
8. validações de faltas, matrículas, vínculo acadêmico, desvinculação/inativação, rematrícula, progressão, encerramento de ano e cálculo de situação acadêmica;
9. endpoints expostos que aceitam payload com `periodo` ou retornam `periodo` em responses;
10. testes unitários, testes de aggregate, testes de projection, testes de handler e regressões existentes;
11. documentação funcional, documentação de API, manual de configuração inicial e qualquer tarefa feita/documento histórico que ainda descreva `periodo` como configurável;
12. compatibilidade com academias escolares, superiores e mistas;
13. replay de eventos antigos e reconstrução das projeções;
14. comportamento para payloads legados contendo `periodo`.

## Checklist obrigatório de validação

### 1. Regra centralizada de período

Confirmar e, se necessário, implementar uma função/constante única responsável por resolver o período fixo por tipo.

Validar que:

- `escolar` resolve sempre para `09_07`;
- `superior` resolve sempre para `10_07`;
- não há cópias soltas de strings mágicas que possam divergir em outros pontos do código;
- chamadas com tipo vazio, desconhecido ou inválido falham com erro claro quando aplicável;
- a regra central é reutilizada por criação, atualização, leitura derivada, cálculo de datas, migração/backfill e validação;
- testes unitários cobrem todos os tipos válidos e inválidos.

### 2. Criação de anos letivos

Auditar todos os fluxos de criação de anos letivos e configurações equivalentes.

Validar que:

- payloads de criação não usam `periodo` como fonte de verdade;
- se o payload antigo ainda aceitar `periodo`, a estratégia adotada seja consistente: rejeitar divergente com erro claro ou ignorar/substituir pelo valor derivado;
- criação de `type="escolar"` grava `periodo="09_07"`, mesmo se o cliente tentar enviar outro valor;
- criação de `type="superior"` grava `periodo="10_07"`, mesmo se o cliente tentar enviar outro valor;
- responses retornam `periodo` apenas como valor fixo/derivado do sistema;
- academias mistas não conseguem criar dois períodos conflitantes para o mesmo tipo;
- importações, batch, jobs ou endpoints administrativos seguem exatamente a mesma regra dos endpoints comuns.

### 3. Atualização de anos letivos

Auditar todos os fluxos de atualização parcial ou total.

Validar que:

- não existe atualização que altere somente `periodo` mantendo o tipo;
- não existe atualização que permita `escolar -> 10_07`;
- não existe atualização que permita `superior -> 09_07`;
- troca de tipo, se existir no domínio, recalcula obrigatoriamente o período pelo novo tipo e valida impactos acadêmicos;
- update parcial que omite `periodo` preserva ou recalcula corretamente o valor fixo;
- update parcial que informa `periodo` divergente é rejeitado ou normalizado conforme a estratégia documentada;
- mensagens de erro deixam claro que período letivo é regra fixa do sistema, não configuração da academia.

### 4. Persistência, migrations e dados legados

Investigar schema, migrations e dados projetados.

Validar que:

- tabelas/projeções de anos letivos não têm defaults configuráveis incompatíveis com a regra fixa;
- constraints, defaults e índices não incentivam períodos arbitrários;
- migrations anteriores que inserem `periodo` foram neutralizadas, corrigidas ou explicadas;
- existe migração corretiva se houver risco real de dados legados com escolar em `10_07` ou superior em `09_07`;
- a migração corretiva é idempotente e segura para reexecução;
- a correção não apaga histórico acadêmico, avaliações, faltas, vínculos ou encerramentos;
- replay de eventos antigos não recria projeções com período inválido;
- scripts operacionais não reintroduzem período configurável.

### 5. Cálculo de intervalo real do ano letivo

Auditar qualquer função que transforme `ano_letivo` + tipo/período em datas reais.

Validar que:

- `escolar` + `2025_2026` resulte em início `2025-09-01` e fim `2026-07-31`;
- `superior` + `2025_2026` resulte em início `2025-10-01` e fim `2026-07-31`;
- o cálculo use a regra central de período por tipo, e não período arbitrário salvo em payload;
- anos letivos como `2026_2027` sejam calculados por parsing seguro do identificador;
- identificadores inválidos falhem com erro claro;
- timezone e fim de mês não gerem exclusões indevidas no último dia letivo;
- testes cubram início, fim, limites inclusivos e casos inválidos.

### 6. Faltas e frequência

Auditar lançamento, atualização, consulta e validação de faltas.

Validar que:

- faltas escolares só sejam aceitas dentro da janela fixa escolar do ano letivo;
- faltas superiores só sejam aceitas dentro da janela fixa superior do ano letivo;
- a validação não usa `periodo` enviado pelo cliente;
- correções de falta em datas-limite (`09-01`, `10-01`, `07-31`) funcionam conforme o tipo;
- tentativas fora da janela retornam erro claro;
- relatórios e projections de frequência não agrupam dados por período configurável antigo.

### 7. Matrículas, vínculos e estudantes

Auditar criação de estudante, vínculo acadêmico, solicitação de matrícula, aprovação de solicitação, rematrícula, desvinculação, inativação e histórico.

Validar que:

- datas de vínculo/matrícula são comparadas contra a janela fixa correta;
- estudantes escolares e superiores em academia mista usam janelas diferentes conforme o tipo aplicável;
- histórico acadêmico não muda retroativamente se o período fixo for recalculado em projeção;
- não há bypass por cadastro direto, solicitação de matrícula, endpoints administrativos ou importação;
- erros de data fora do período informam o tipo e a janela esperada.

### 8. Encerramento, progressão e regras acadêmicas dependentes

Auditar finalização de ano letivo, bloqueio de retrocessos, progressão escolar/superior, aprovação/reprovação e transição de ano/período acadêmico.

Validar que:

- encerramento de ano letivo usa datas derivadas da regra fixa;
- progressão não usa um `periodo` configurado por academia;
- superior continua respeitando progressão semestral quando aplicável, mas o ano letivo superior continua com período fixo `10_07`;
- escola continua respeitando anos acadêmicos escolares, mas o ano letivo escolar continua com período fixo `09_07`;
- não há regressão em tarefas já feitas relacionadas a encerramento, progressão e anos acadêmicos.

### 9. API, contratos e compatibilidade de payloads

Auditar todos os contratos expostos.

Validar que:

- documentação de request não ensina o cliente a configurar `periodo`;
- exemplos válidos não incluem `type="escolar"` com `periodo="10_07"`;
- exemplos válidos não incluem `type="superior"` com `periodo="09_07"`;
- se `periodo` aparece em response, está descrito como derivado/fixo/read-only;
- erros de validação para payload legado são documentados quando a estratégia for rejeição;
- clientes antigos que enviem `periodo` recebem comportamento consistente, previsível e testado;
- OpenAPI, documentação Markdown, manuais e exemplos estão sincronizados com o código real.

### 10. Testes obrigatórios

Criar ou ajustar testes cobrindo, no mínimo:

- função central `escolar -> 09_07`;
- função central `superior -> 10_07`;
- tipo inválido retorna erro claro;
- criação de ano letivo escolar gravando/retornando `09_07`;
- criação de ano letivo superior gravando/retornando `10_07`;
- tentativa de enviar período incompatível em criação;
- tentativa de alterar período incompatível em atualização;
- update parcial sem `periodo` preservando/recalculando o fixo correto;
- cálculo de intervalo `2025_2026` escolar como `2025-09-01` a `2026-07-31`;
- cálculo de intervalo `2025_2026` superior como `2025-10-01` a `2026-07-31`;
- validação de falta no primeiro dia e último dia letivo escolar;
- validação de falta fora da janela escolar;
- validação de falta no primeiro dia e último dia letivo superior;
- validação de falta fora da janela superior;
- matrícula/vínculo em academia mista usando tipo correto;
- replay/projeção de evento antigo não recriando período inválido;
- documentação/contratos sem exemplos de período configurável.

## Busca ampla obrigatória

Fazer busca ampla e classificar cada ocorrência encontrada como válida, histórica/documental aceitável ou bug ativo. No mínimo, buscar por:

- `periodo`
- `período`
- `ano_letivo`
- `anos_letivos`
- `09_07`
- `10_07`
- `2025_2026`
- `escolar`
- `superior`
- `Periodo`
- `period`
- `start_date`
- `end_date`
- `data_inicio`
- `data_fim`

Não basta listar ocorrências: cada ocorrência relevante deve ser analisada no contexto do arquivo e do fluxo de execução.

## Documentação obrigatória

Confirmar e corrigir, se necessário, que a documentação esteja de acordo com a nova versão do código.

A documentação deve deixar explícito que:

- `periodo` é fixo, imutável e derivado do tipo de ano letivo;
- escolas usam sempre `09_07`;
- superior usa sempre `10_07`;
- academias e admins não configuram período letivo;
- payloads de criação/edição não devem enviar `periodo` como configuração;
- responses podem expor `periodo` somente como valor read-only/derivado;
- validações acadêmicas usam a janela real derivada do identificador do ano letivo e do tipo;
- dados legados incompatíveis foram corrigidos ou são bloqueados por validação clara.

Verificar no mínimo:

- documentação funcional principal;
- documentação de API;
- manual de configuração inicial da academia;
- tarefas feitas relacionadas a anos letivos, encerramento, progressão, faltas, matrículas e curso superior;
- exemplos JSON, tabelas e textos explicativos.

## Correções esperadas quando houver divergência

Se a auditoria encontrar qualquer divergência, implementar a correção no mesmo ciclo de depuração. Exemplos de correções esperadas:

1. extrair strings mágicas para função central de resolução de período;
2. remover `periodo` de DTOs de escrita ou validar/rejeitar divergência;
3. normalizar handlers para nunca persistirem período arbitrário;
4. ajustar aggregate/eventos/projections para recalcular ou validar o período fixo;
5. criar migration idempotente para corrigir registros legados incompatíveis;
6. atualizar cálculo de intervalo letivo para derivar mês inicial/final da regra central;
7. ajustar validações de faltas, matrículas, vínculos e encerramento;
8. adicionar testes regressivos que falhem na implementação antiga e passem na nova;
9. atualizar documentação e exemplos para remover qualquer indício de período configurável.

## Critério de aceite final

A depuração só pode ser considerada concluída quando:

- todos os itens do checklist forem verificados;
- cada bug encontrado for corrigido no código, nas migrações, nos testes e/ou na documentação;
- a suíte relevante de testes passar;
- a documentação estiver sincronizada com o comportamento real do backend;
- não restar nenhum fluxo que aceite `periodo` como configuração arbitrária;
- não restar nenhum exemplo/documento que ensine período letivo configurável;
- a tarefa original tiver o título interno alterado para `# Implementar períodos fixos e imutáveis dos anos letivos (feito)`;
- o arquivo da tarefa original for movido de `docs/Lista de tarefas/` para `docs/Tarefas feitas/`, mantendo o nome do arquivo sem adicionar `(feito)`.
