---
modificado: 2026-06-29 00:00
criado: 2026-06-29 00:00
---
# Remover PATCH de anos acadêmicos e corrigir validação de academia fundamental

## Objetivo

Remover por segurança a rota **`PATCH /academia/anos-academicos`** do contrato público e do roteamento do backend, mantendo para anos acadêmicos apenas as operações explícitas de **consulta**, **adição** e **remoção**:

- `GET /academia/anos-academicos`
- `POST /academia/anos-academicos`
- `DELETE /academia/anos-academicos`

A academia não deve mais substituir listas completas de anos acadêmicos por atualização parcial ou total via `PATCH`. O fluxo permitido passa a ser estritamente incremental e auditável: a academia pode ler anos acadêmicos existentes, adicionar novos anos acadêmicos permitidos e deletar anos acadêmicos permitidos quando a remoção for segura.

A tarefa também deve corrigir o erro indevido ocorrido ao tentar adicionar e deletar ano acadêmico em uma academia fundamental cadastrada como `nivel='escola'` e `nivel_escolar='fundamental'`:

> Esta academia não pode gerenciar anos do ensino fundamental porque o nível cadastrado é nivel='escola' e nivel_escolar='fundamental'. Somente academias escolares com nivel_escolar 'fundamental' ou 'misto' podem alterar anos fundamentais.

Esse cenário deve ser tratado como **permitido**, não como nível incompatível.

## Contexto e motivação

A rota `PATCH /academia/anos-academicos` permite substituir a configuração de anos acadêmicos em uma única operação. Para o domínio atual, esse comportamento aumenta o risco de remoção acidental, alteração ampla não intencional e inconsistência entre anos configurados, estudantes, matérias, turmas, notas, faltas, avaliações finais e cursos.

Por segurança, a API deve abandonar o modelo de substituição e aceitar apenas operações com intenção explícita:

- `GET` para leitura sem efeito colateral;
- `POST` para adicionar anos acadêmicos ou períodos;
- `DELETE` para remover anos acadêmicos ou períodos, somente quando não houver uso ativo nem violação de regra mínima.

Além disso, a validação de anos fundamentais está rejeitando incorretamente academias escolares fundamentais. Uma academia com `nivel='escola'` e `nivel_escolar='fundamental'` precisa poder gerenciar anos do ensino fundamental, desde que respeite todas as validações de segurança.

## Regra de negócio a implementar

### Regra geral

- Remover completamente a rota `PATCH /academia/anos-academicos`.
- Não criar rota substituta para atualização em massa.
- Não manter alias, fallback, compatibilidade retroativa, feature flag, handler oculto ou resposta especial para `PATCH`.
- O contrato de anos acadêmicos deve expor somente `GET`, `POST` e `DELETE`.
- `POST` deve adicionar apenas anos/períodos permitidos e não deve remover itens existentes.
- `DELETE` deve remover apenas anos/períodos permitidos e não deve adicionar itens novos.
- `GET` deve continuar sendo somente leitura.
- Toda alteração deve continuar sendo vinculada à academia autenticada, ativa e autorizada.
- Administradores podem consultar conforme a regra atual, mas não devem ganhar permissão de escrita por essa tarefa.

### Segurança obrigatória

- Cursos nunca devem ficar sem anos acadêmicos ou semestres após operações de escrita.
- Nenhuma operação de escrita pode aceitar `codigo_academia` para alterar dados de outra academia.
- Escritas devem usar exclusivamente a academia autenticada no token/sessão.
- A academia precisa estar ativa antes de qualquer escrita.
- A remoção deve ser bloqueada quando o ano/período estiver em uso por estudantes, matérias, turmas, notas, faltas, avaliações finais, categorias ou qualquer outra projeção/regra dependente existente.
- A remoção deve preservar o mínimo obrigatório de anos acadêmicos para escolas fundamentais e mistas.
- O backend não deve aceitar payloads ambíguos que misturem intenção de adicionar e remover.
- O backend não deve aceitar campos desconhecidos se o padrão atual da API permitir rejeição; se o padrão atual ainda aceitar campos extras, esta rota deve rejeitar explicitamente campos perigosos como `substituir`, `replace`, `patch`, `set`, `update`, `codigo_academia` em escrita e equivalentes.
- Erros devem ser claros, estruturados e não devem vazar dados internos além do necessário para o cliente corrigir o payload.
- A documentação deve deixar explícito que substituição em massa de anos acadêmicos foi removida por segurança.

## Casos por nível da academia

### Academia escolar fundamental

Condição cadastral esperada:

- `nivel='escola'`
- `nivel_escolar='fundamental'`

Comportamento obrigatório:

- Pode usar `GET /academia/anos-academicos` para consultar seus anos fundamentais.
- Pode usar `POST /academia/anos-academicos` com `type='fundamental'` para adicionar anos fundamentais válidos.
- Pode usar `DELETE /academia/anos-academicos` com `type='fundamental'` para remover anos fundamentais válidos, desde que a remoção seja segura.
- Não pode gerenciar anos de ensino médio.
- Não pode gerenciar anos/períodos de ensino superior.
- Não pode remover todos os anos fundamentais; deve permanecer pelo menos um ano acadêmico ativo quando a regra do domínio exigir anos para fundamental.
- Não deve receber erro de nível incompatível ao adicionar ou deletar anos fundamentais.

Correção obrigatória do bug:

- A validação que autoriza fundamental deve considerar válido `nivel='escola'` com `nivel_escolar='fundamental'`.
- O erro abaixo não pode ocorrer para academia fundamental válida ao operar `type='fundamental'`:
  - `Esta academia não pode gerenciar anos do ensino fundamental porque o nível cadastrado é nivel='escola' e nivel_escolar='fundamental'. Somente academias escolares com nivel_escolar 'fundamental' ou 'misto' podem alterar anos fundamentais.`
- Esse erro só pode aparecer quando a academia realmente não for escolar fundamental nem escolar mista.

### Academia escolar médio

#### Validação obrigatória de sequência dos anos do médio

Os anos acadêmicos de um curso do ensino médio devem ser obrigatoriamente sequenciais, em ordem crescente, começando sempre por `1_ano_medio`. A lista válida deve representar um prefixo contínuo da sequência permitida, por exemplo:

- válido: [`1_ano_medio`];
- válido: [`1_ano_medio`, `2_ano_medio`, `3_ano_medio`];
- inválido: [`1_ano_medio`, `3_ano_medio`, `4_ano_medio`], pois pula `2_ano_medio`;
- inválido: [`2_ano_medio`, `3_ano_medio`], pois não começa por `1_ano_medio`;
- inválido: lista vazia, pois cursos nunca devem ficar sem anos acadêmicos.

`POST` e `DELETE` com `type='medio'` devem validar o estado final do curso após a operação. Nunca pode existir lacuna, salto, ano vazio entre anos configurados ou ordem fora da sequência crescente esperada.

Condição cadastral esperada:

- `nivel='escola'`
- `nivel_escolar='medio'`

Comportamento obrigatório:

- Pode usar `GET /academia/anos-academicos` para consultar dados disponíveis conforme o contrato atual.
- Não pode usar `POST` ou `DELETE` com `type='fundamental'`.
- Não deve ter `anos_academicos` diretamente na academia para fundamental.
- A gestão de anos do médio deve permanecer vinculada ao curso médio informado por `curso_id`, conforme a regra atual do sistema.
- Pode usar `POST /academia/anos-academicos` com `type='medio'` apenas para adicionar anos acadêmicos válidos ao curso médio ativo e pertencente à academia, preservando sequência crescente contínua iniciada em `1_ano_medio`.
- Pode usar `DELETE /academia/anos-academicos` com `type='medio'` apenas para remover anos acadêmicos seguros do curso médio ativo e pertencente à academia, sem criar lacunas e sem deixar o curso sem anos acadêmicos.
- Não pode gerenciar períodos/anos de ensino superior por essa rota.
- Não pode usar `PATCH` para substituir a lista completa de anos do curso médio.

### Academia escolar mista

Condição cadastral esperada:

- `nivel='escola'`
- `nivel_escolar='misto'`

Comportamento obrigatório:

- Pode usar `GET /academia/anos-academicos` para consultar anos fundamentais da academia e anos de cursos médios conforme o contrato atual.
- Pode usar `POST` com `type='fundamental'` para adicionar anos fundamentais válidos diretamente à academia.
- Pode usar `DELETE` com `type='fundamental'` para remover anos fundamentais válidos, desde que a remoção seja segura e reste pelo menos um ano fundamental quando exigido.
- Pode usar `POST` com `type='medio'` para adicionar anos acadêmicos válidos a curso médio ativo e pertencente à academia.
- Pode usar `DELETE` com `type='medio'` para remover anos acadêmicos seguros de curso médio ativo e pertencente à academia.
- Não pode usar `type='superior'`.
- Não pode usar `PATCH` para substituir anos fundamentais nem anos do médio.
- A permissão para fundamental e médio deve ser avaliada de forma separada para evitar que uma operação de médio altere fundamental ou o inverso.

### Academia superior

#### Validação obrigatória de semestres/períodos

Cursos superiores nunca podem ficar sem anos acadêmicos, períodos ou semestres derivados. A adição e a remoção direta de anos acadêmicos em curso superior por `/academia/anos-academicos` deve ser bloqueada quando tentar alterar manualmente a lista derivada. Em curso superior, a fonte de verdade para essa configuração é o número de semestres em `periodos`; portanto, qualquer mudança na quantidade de anos acadêmicos/semestres deve ocorrer pela alteração de `periodos`, respeitando a regra existente do curso.

Condição cadastral esperada:

- `nivel='superior'`
- `nivel_escolar` ausente ou `null`

Comportamento obrigatório:

- Pode usar `GET /academia/anos-academicos` para consultar anos/períodos dos cursos superiores conforme o contrato atual.
- Não pode usar `type='fundamental'`.
- Não pode usar `type='medio'`.
- Não pode adicionar anos acadêmicos/semestres diretamente em curso superior por `POST /academia/anos-academicos`; a alteração deve ser feita pelo número de semestres em `periodos`.
- Não pode remover anos acadêmicos/semestres diretamente em curso superior por `DELETE /academia/anos-academicos`; a alteração deve ser feita pelo número de semestres em `periodos` e somente quando segura.
- Qualquer validação de curso superior deve garantir que o curso não fique sem períodos/semestres derivados.
- Não pode usar `PATCH` para substituir todos os anos/períodos do curso superior.
- Deve continuar respeitando a diferença entre ano acadêmico superior derivado e período/semestre quando a regra existente fizer essa distinção.

## Escopo dos ajustes necessários

### Rotas e handlers

Remover do servidor a rota:

- `PATCH /academia/anos-academicos`

Remover ou desativar definitivamente o handler de atualização/substituição associado, incluindo qualquer função, request struct, validação, documentação de binding e teste que exista apenas para o fluxo de `PATCH`.

A remoção não deve ser implementada como handler que retorna erro amigável. A rota deve deixar de estar registrada.

### Contrato de payload

Garantir que os payloads de escrita fiquem separados por intenção:

- `POST` adiciona anos/períodos.
- `DELETE` remove anos/períodos.
- Nenhuma rota substitui a lista inteira.

Se existir request compartilhado entre `POST`, `PATCH` e `DELETE`, separar ou simplificar para evitar que campos de substituição continuem aceitos por acidente.

### Validação de nível

Revisar a validação de permissão por `nivel` e `nivel_escolar` para garantir que:

- `nivel='escola'` + `nivel_escolar='fundamental'` pode operar `type='fundamental'`.
- `nivel='escola'` + `nivel_escolar='misto'` pode operar `type='fundamental'` e `type='medio'`.
- `nivel='escola'` + `nivel_escolar='medio'` pode operar `type='medio'` somente via curso médio válido.
- `nivel='superior'` pode operar `type='superior'` somente via curso superior válido.
- Qualquer combinação incompatível deve falhar com erro estruturado e mensagem coerente.

A correção deve tratar possíveis diferenças de representação como ponteiro, string vazia, espaços e capitalização somente se esse tipo de normalização já for padrão no projeto; não introduzir compatibilidade perigosa que aceite valores inválidos fora do domínio.

### Dependências, sequência e bloqueios de remoção

Manter e reforçar as validações que impedem remoções inseguras. Antes de remover um ano/período, verificar uso ativo conforme as projeções existentes e validar o estado final da entidade afetada:

- cursos nunca podem ficar com lista vazia de anos acadêmicos, períodos ou semestres;
- cursos médios devem permanecer com anos sequenciais, em ordem crescente, começando em `1_ano_medio`, sem pular ou deixar vazio entre anos;
- cursos superiores não devem aceitar adição/remoção direta de anos acadêmicos/semestres nessa rota, pois a quantidade deve ser derivada de `periodos`.

Também deve ser verificado uso ativo, incluindo no mínimo:

- estudantes;
- turmas;
- matérias;
- notas;
- faltas;
- avaliações finais;
- categorias de nota configuradas por ano;
- cursos e períodos quando aplicável;
- qualquer outra entidade do domínio que referencie o ano/período.

Se já existir função central para detectar uso, reutilizar e cobrir todos os tipos. Se não existir, criar validação central para evitar regras divergentes entre fundamental, médio e superior.

### Documentação

Atualizar a documentação da API e a documentação funcional para remover:

- seção de `PATCH /academia/anos-academicos`;
- exemplos de substituição de lista;
- menções de que academias podem substituir anos acadêmicos;
- qualquer orientação que incentive alteração em massa.

A documentação final deve declarar que somente `GET`, `POST` e `DELETE` existem para `/academia/anos-academicos`.

### Testes

Atualizar ou criar testes cobrindo:

- ausência da rota `PATCH`;
- permissão correta para academia fundamental;
- permissão correta para academia médio;
- permissão correta para academia mista;
- permissão correta para academia superior;
- bloqueios de remoção insegura;
- erros estruturados para combinações incompatíveis.

## Compatibilidade com clientes existentes

Não deve haver compatibilidade retroativa para `PATCH /academia/anos-academicos`.

- Não manter endpoint antigo retornando `410 Gone` apenas para preservar contrato.
- Não aceitar payload antigo de substituição via `POST` ou `DELETE`.
- Não criar alias como `PUT`, `POST /replace`, `POST /atualizar`, `POST /sync` ou equivalente.
- Não aceitar campo que simule patch/substituição em payloads de `POST` ou `DELETE`.

Clientes integrados devem migrar para o fluxo explícito de adicionar ou deletar itens individualmente ou por lista segura, conforme o contrato de `POST` e `DELETE`.

## Fora de escopo

- Criar nova rota de substituição em massa.
- Criar fluxo administrativo para editar anos de qualquer academia.
- Alterar o modelo cadastral de `nivel` e `nivel_escolar`.
- Permitir que escola fundamental gerencie médio.
- Permitir que escola médio gerencie fundamental.
- Permitir que escola mista gerencie superior.
- Permitir que superior gerencie fundamental ou médio.
- Remover validações de uso ativo para facilitar deleção.
- Migrar dados históricos de anos acadêmicos.

## Validações obrigatórias após a mudança

- `GET /academia/anos-academicos` deve continuar registrado e funcional.
- `POST /academia/anos-academicos` deve continuar registrado e funcional para combinações permitidas.
- `DELETE /academia/anos-academicos` deve continuar registrado e funcional para combinações permitidas.
- `PATCH /academia/anos-academicos` não deve estar registrado.
- Cursos nunca devem ficar sem anos acadêmicos/semestres.
- Curso médio deve manter anos sequenciais em ordem crescente iniciando sempre em `1_ano_medio`, sem lacunas como [`1_ano_medio`, `3_ano_medio`, `4_ano_medio`].
- Curso superior deve bloquear adição/remoção direta de anos acadêmicos/semestres; alterações devem ocorrer pelo número de semestres em `periodos`.
- Academia `nivel='escola'` e `nivel_escolar='fundamental'` deve conseguir adicionar ano fundamental válido.
- Academia `nivel='escola'` e `nivel_escolar='fundamental'` deve conseguir deletar ano fundamental válido quando a remoção for segura.
- Academia fundamental não deve receber erro de nível incompatível ao operar `type='fundamental'`.
- Academia médio deve ser bloqueada ao tentar operar `type='fundamental'`.
- Academia mista deve poder operar `type='fundamental'` e `type='medio'` conforme regras específicas de cada tipo.
- Academia superior deve ser bloqueada ao tentar operar `type='fundamental'` ou `type='medio'`.
- Escola, seja fundamental, médio ou mista, deve ser bloqueada ao tentar operar `type='superior'`.
- Remoção de ano/período em uso deve continuar bloqueada.
- Remoção que deixaria escola fundamental/mista sem nenhum ano fundamental obrigatório deve continuar bloqueada.
- Payloads de escrita com intenção de substituição devem falhar.

## Fluxo operacional proposto

1. Localizar o registro da rota `PATCH /academia/anos-academicos` no servidor.
2. Remover o registro da rota e o handler de atualização/substituição se ele não for mais usado.
3. Revisar requests e validações compartilhadas para impedir aceitação acidental de payload de substituição.
4. Corrigir a validação de `type='fundamental'` para aceitar academia `nivel='escola'` com `nivel_escolar='fundamental'` ou `nivel_escolar='misto'`.
5. Revisar validações de `medio`, `misto` e `superior` para garantir que cada nível só gerencie seu próprio escopo.
6. Implementar validação de estado final para impedir curso sem anos acadêmicos/semestres, impedir lacunas na sequência do médio e bloquear adição/remoção direta de anos acadêmicos/semestres de curso superior fora de `periodos`.
7. Garantir que `POST` não remove anos existentes e `DELETE` não adiciona anos novos.
8. Reforçar bloqueios de remoção de anos/períodos em uso.
9. Atualizar documentação removendo `PATCH` e explicando o fluxo seguro `GET`/`POST`/`DELETE`.
10. Atualizar testes automatizados e fixtures para não dependerem de `PATCH`.
11. Executar testes e verificação de rotas para confirmar que o contrato final está seguro.

## Impactos esperados

- A API deixa de permitir substituição em massa de anos acadêmicos.
- O contrato de `/academia/anos-academicos` fica mais restrito e previsível.
- Academias passam a operar anos acadêmicos apenas por leitura, adição e deleção segura.
- Academias fundamentais voltam a conseguir adicionar e deletar anos fundamentais válidos.
- O risco de remoção acidental ou alteração ampla de anos acadêmicos é reduzido.
- Clientes que usavam `PATCH` precisarão migrar para `POST` e `DELETE`.

## Documentação da API

Atualizar a documentação para refletir que:

- `PATCH /academia/anos-academicos` não existe mais;
- os métodos permitidos são somente `GET`, `POST` e `DELETE`;
- `POST` adiciona anos/períodos;
- `DELETE` remove anos/períodos com validações de segurança;
- não há operação de substituir lista completa;
- academias fundamentais com `nivel='escola'` e `nivel_escolar='fundamental'` podem gerenciar `type='fundamental'`;
- academias mistas podem gerenciar `type='fundamental'` e `type='medio'` nos seus respectivos escopos;
- academias médio gerenciam apenas `type='medio'` por curso médio;
- academias superiores não adicionam nem removem diretamente anos acadêmicos/semestres por essa rota; alterações de quantidade devem ocorrer pelo número de semestres em `periodos`;
- cursos nunca podem ficar sem anos acadêmicos/semestres;
- cursos médios devem manter anos sequenciais crescentes começando em `1_ano_medio`, sem lacunas.

## Testes recomendados

### Rotas

- `GET /academia/anos-academicos` deve estar registrado.
- `POST /academia/anos-academicos` deve estar registrado.
- `DELETE /academia/anos-academicos` deve estar registrado.
- `PATCH /academia/anos-academicos` não deve estar registrado.

### Fundamental

- Academia `nivel='escola'`, `nivel_escolar='fundamental'`, `POST type='fundamental'` com ano válido: deve passar.
- Academia `nivel='escola'`, `nivel_escolar='fundamental'`, `DELETE type='fundamental'` com ano válido sem uso: deve passar.
- Academia `nivel='escola'`, `nivel_escolar='fundamental'`, `DELETE type='fundamental'` removendo todos os anos: deve falhar.
- Academia `nivel='escola'`, `nivel_escolar='fundamental'`, `POST/DELETE type='medio'`: deve falhar.
- Academia `nivel='escola'`, `nivel_escolar='fundamental'`, `POST/DELETE type='superior'`: deve falhar.

### Médio

- Academia `nivel='escola'`, `nivel_escolar='medio'`, `POST type='medio'` com `curso_id` ativo da academia e estado final sequencial desde `1_ano_medio`: deve passar.
- Academia `nivel='escola'`, `nivel_escolar='medio'`, `DELETE type='medio'` com `curso_id` ativo, ano sem uso e estado final sem lacunas nem lista vazia: deve passar.
- Academia `nivel='escola'`, `nivel_escolar='medio'`, `POST/DELETE type='medio'` que deixaria [`1_ano_medio`, `3_ano_medio`, `4_ano_medio`] ou qualquer sequência com lacuna: deve falhar.
- Academia `nivel='escola'`, `nivel_escolar='medio'`, `DELETE type='medio'` removendo todos os anos do curso: deve falhar.
- Academia `nivel='escola'`, `nivel_escolar='medio'`, `POST/DELETE type='fundamental'`: deve falhar.
- Academia `nivel='escola'`, `nivel_escolar='medio'`, `POST/DELETE type='superior'`: deve falhar.
- Academia `nivel='escola'`, `nivel_escolar='medio'`, `POST/DELETE type='medio'` com curso inativo ou de outra academia: deve falhar.

### Misto

- Academia `nivel='escola'`, `nivel_escolar='misto'`, `POST type='fundamental'` com ano válido: deve passar.
- Academia `nivel='escola'`, `nivel_escolar='misto'`, `DELETE type='fundamental'` com ano sem uso: deve passar.
- Academia `nivel='escola'`, `nivel_escolar='misto'`, `POST type='medio'` com curso médio ativo da academia: deve passar.
- Academia `nivel='escola'`, `nivel_escolar='misto'`, `DELETE type='medio'` com ano sem uso: deve passar.
- Academia `nivel='escola'`, `nivel_escolar='misto'`, `POST/DELETE type='superior'`: deve falhar.
- Operação de médio em academia mista não deve alterar anos fundamentais.
- Operação de fundamental em academia mista não deve alterar anos do curso médio.

### Superior

- Academia `nivel='superior'`, `nivel_escolar=null`, `POST type='superior'` tentando adicionar ano acadêmico/semestre diretamente: deve falhar e orientar alteração via `periodos`.
- Academia `nivel='superior'`, `nivel_escolar=null`, `DELETE type='superior'` tentando remover ano acadêmico/semestre diretamente: deve falhar e orientar alteração via `periodos`.
- Alteração de `periodos` em curso superior deve manter pelo menos um período/semestre derivado e respeitar dependências existentes.
- Academia `nivel='superior'`, `nivel_escolar=null`, `POST/DELETE type='fundamental'`: deve falhar.
- Academia `nivel='superior'`, `nivel_escolar=null`, `POST/DELETE type='medio'`: deve falhar.
- Academia `nivel='superior'`, `nivel_escolar` preenchido indevidamente: deve falhar ou ser tratada conforme validação cadastral existente, sem liberar escopo escolar.

### Segurança e regressão

- `POST` com campos de substituição em massa deve falhar.
- `DELETE` com campos de substituição em massa deve falhar.
- Escrita com `codigo_academia` tentando alterar outra academia deve falhar ou ignorar o campo conforme política segura definida, preferencialmente falhar.
- Remover ano/período usado por estudante ativo deve falhar.
- Remover ano/período usado por matéria ativa deve falhar.
- Remover ano/período usado por turma ativa deve falhar.
- Remover ano/período usado por nota, falta ou avaliação final deve falhar.
- Busca textual por `PATCH /academia/anos-academicos` deve retornar apenas documentação histórica inevitável ou a própria tarefa, nunca registro ativo de rota ou contrato atual.
