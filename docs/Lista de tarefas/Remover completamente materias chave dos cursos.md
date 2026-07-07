# Remover completamente o conceito de matérias-chave dos cursos

## Contexto

O backend ainda possui suporte ativo e documentação relacionada ao conceito de `materias_chave` em cursos médios. Esse conceito já passou por mudanças anteriores, saindo da regra de avaliação final para o curso médio, mas a nova decisão de produto é mais ampla: **remover completamente o conceito de matérias-chave de um curso**.

Esta tarefa deve orientar uma implementação de remoção total, sem manter compatibilidade retroativa, aliases, campos aceitos silenciosamente, rotas legadas, DTOs antigos, validações condicionais ou documentação que sugira que a funcionalidade ainda existe.

## Objetivo principal

Eliminar integralmente do sistema o conceito de matérias-chave de curso.

Após a implementação:

- cursos de qualquer nível não aceitam, persistem, retornam nem calculam `materias_chave`;
- não existe rota para configurar matérias-chave de curso;
- avaliação final, progressão, pendências e relatórios não dependem de matérias-chave;
- contratos públicos não aceitam `materias_chave`, `materiasChave`, `MateriasChave` nem variações como aliases;
- migrations, projections, eventos, DTOs, handlers, testes e documentação deixam claro que a funcionalidade foi removida, não apenas ocultada;
- não há suporte legado para converter, ignorar ou tolerar payloads antigos contendo matérias-chave.

## Decisão de produto

A implementação deve tratar matérias-chave como um conceito removido do domínio.

Não é permitido:

- manter campo opcional em request ou response;
- aceitar `materias_chave` e ignorar silenciosamente;
- aceitar `materiasChave` como alias de compatibilidade;
- manter rota legada retornando sucesso sem efeito;
- manter structs, tipos, helpers ou métodos com nomes relacionados a matérias-chave;
- manter branch de regra de negócio que diferencie matéria-chave de matéria não-chave;
- manter documentação dizendo que matérias-chave pertencem ao curso, à regra ou a qualquer outro agregado;
- criar migração que apenas esconda o campo mantendo o conceito disponível para uso futuro.

Payloads contendo campos de matérias-chave em endpoints ativos devem ser rejeitados com erro claro de contrato inválido, exceto quando o padrão real do endpoint já rejeitar campos desconhecidos automaticamente. Em ambos os casos, não deve haver compatibilidade silenciosa.

## Escopo obrigatório de implementação

### 1. Rotas e handlers

Remover completamente a rota específica de configuração de matérias-chave de curso, incluindo o registro no servidor e o handler correspondente.

Auditar e ajustar, no mínimo:

- `PUT /academia/curso/:id/materias-chave` ou rota equivalente existente;
- handlers de criação, edição, leitura e listagem de cursos;
- handlers de avaliação final;
- handlers ou jobs de importação/batch que criem ou atualizem cursos;
- qualquer endpoint que ainda documente, aceite ou exponha `materias_chave`.

A remoção deve garantir que não exista endpoint ativo que permita configurar matérias-chave.

### 2. DTOs, contratos e validações

Remover de todos os contratos públicos e internos:

- `materias_chave`;
- `materiasChave`;
- `MateriasChave`;
- tipos como configuração de matérias-chave por ano;
- validadores específicos de matérias-chave;
- mensagens de erro que orientem configurar matérias-chave.

Se algum endpoint receber payload com campos desconhecidos e o padrão atual for rejeitar esses campos, manter esse comportamento. Se o padrão atual aceitar campos extras, avaliar se o endpoint afetado precisa passar a rejeitar explicitamente `materias_chave` para evitar compatibilidade silenciosa.

### 3. Domínio, eventos e projections

Remover o conceito de todas as camadas de domínio e leitura.

Auditar e ajustar, no mínimo:

- aggregates de curso;
- eventos de criação, atualização e configuração de curso;
- event handlers/projections de curso;
- snapshots ou serializações de curso;
- queries e repositórios que leem cursos;
- estruturas auxiliares usadas por avaliação final;
- testes que montam curso médio com matérias-chave.

Não deve restar evento novo emitido com matérias-chave. Eventos históricos só podem permanecer como dados antigos no ledger, mas o código ativo não deve depender deles nem oferecer alias de reconstrução que mantenha o conceito no modelo público atual.

### 4. Banco de dados, migrations e rebuild de projeções

Criar migration para remover colunas, índices, constraints ou estruturas persistentes relacionadas a matérias-chave de curso, caso existam.

A migration deve:

- remover colunas como `materias_chave`, se existirem em projections ou tabelas ativas;
- remover índices ou constraints associados;
- ser idempotente quando o padrão de migrations do projeto exigir;
- não transformar dados antigos em outro campo equivalente;
- não criar aliases ou colunas substitutas.

Também revisar migrations históricas apenas quando necessário para entender o modelo. Migrations antigas já aplicadas podem continuar existindo como histórico, mas não devem ser usadas como justificativa para manter suporte ativo no código atual.

### 5. Avaliação final e progressão acadêmica

Remover qualquer regra que use matérias-chave para aprovar, reprovar, bloquear progressão ou diferenciar matérias.

Após a mudança:

- a avaliação final do médio não busca matérias-chave no curso;
- não há decisão de aprovação direta baseada em subconjunto de matérias-chave;
- matérias disciplinares devem ser tratadas conforme as regras gerais atuais de avaliação final, pendências e conclusão;
- logs, snapshots e auditoria não devem registrar lista de matérias-chave usada;
- erros de configuração ausente de matérias-chave deixam de existir.

Se a regra de avaliação final atual precisar de um critério substituto para médio, a implementação deve usar o comportamento geral já existente para matérias aplicáveis, pendências e fórmulas, sem recriar o conceito de matéria-chave com outro nome.

### 6. Testes

Atualizar ou remover testes que assumem matérias-chave.

Criar ou ajustar testes para cobrir, no mínimo:

- criação de curso médio sem `materias_chave`;
- edição de curso médio sem `materias_chave`;
- rejeição ou tratamento como campo desconhecido quando `materias_chave` for enviado para endpoints de curso;
- inexistência da rota de configuração de matérias-chave;
- respostas de curso sem `materias_chave`;
- avaliação final média sem dependência de matérias-chave;
- ausência de snapshots/auditoria contendo lista de matérias-chave;
- rebuild de projection sem reintroduzir `materias_chave` no modelo público atual.

Também remover fixtures, builders e helpers que criem matérias-chave por padrão.

### 7. Busca e auditoria obrigatória

Antes de concluir a implementação, fazer busca ampla e classificar todas as ocorrências encontradas.

Pesquisar, no mínimo:

```bash
rg -n "materias_chave|materiasChave|MateriasChave|matérias-chave|materias-chave|materias chave|matérias chave|chave" .
```

Para cada ocorrência relacionada ao conceito removido, classificar como uma das opções:

- código ativo a remover;
- teste/fixture a atualizar;
- documentação atual a corrigir;
- migration histórica aceitável;
- tarefa histórica em `docs/Tarefas feitas` aceitável apenas como registro do passado;
- falso positivo sem relação com matérias-chave de curso.

A tarefa só deve ser considerada concluída quando não houver ocorrência ativa que mantenha suporte ao conceito.

## Documentação obrigatória

Atualizar toda documentação funcional e de API impactada.

A atualização deve incluir obrigatoriamente:

- `docs/Manual de Configuração Inicial da Academia.md`;
- documentação de cursos;
- documentação de matérias disciplinares, se mencionar matérias-chave;
- documentação de avaliação final;
- exemplos de payload de criação e edição de curso;
- exemplos de resposta de curso;
- fluxos de configuração inicial de academia;
- orientações operacionais que antes mandavam configurar matérias-chave.

O manual inicial da academia deve orientar o fluxo sem matérias-chave. Em especial, remover qualquer passo que diga para configurar matérias-chave em cursos médios e garantir que o fluxo passe a ser:

1. configurar anos letivos e anos acadêmicos;
2. criar cursos conforme o nível da academia;
3. criar matérias disciplinares associadas aos cursos/anos quando aplicável;
4. configurar categorias de nota e regras de avaliação final conforme o modelo vigente;
5. matricular estudantes e operar notas/faltas sem etapa de matérias-chave.

A documentação não deve dizer que matérias-chave foram movidas para outro lugar. Deve dizer apenas que o conceito não faz parte do modelo atual.

## Critérios de aceite

A implementação será aceita somente se todos os pontos abaixo forem verdadeiros:

- não existe rota ativa para configurar matérias-chave de curso;
- nenhum request público aceita `materias_chave`, `materiasChave` ou equivalente;
- nenhum response público de curso, regra ou avaliação final expõe matérias-chave;
- não existe código ativo que diferencie matéria-chave de matéria não-chave;
- avaliação final média funciona sem buscar matérias-chave no curso;
- migrations removem persistência ativa de matérias-chave;
- testes e fixtures não dependem de matérias-chave;
- `docs/Manual de Configuração Inicial da Academia.md` está atualizado;
- documentação atual não instrui configurar matérias-chave;
- ocorrências restantes do termo estão restritas a histórico aceitável, como migrations antigas ou tarefas já concluídas, e foram justificadas na entrega.

## Comandos mínimos de validação esperados

Executar, no mínimo:

```bash
rg -n "materias_chave|materiasChave|MateriasChave|matérias-chave|materias-chave|materias chave|matérias chave" .
go test ./...
```

Se algum comando não puder ser executado por limitação de ambiente, registrar a limitação explicitamente na entrega.
