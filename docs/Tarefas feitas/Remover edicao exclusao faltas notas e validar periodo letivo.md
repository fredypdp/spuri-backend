---
criado: 2026-07-08 00:00
origem: solicitação do usuário
status: pronto_para_implementacao
---

# Remover edição/exclusão de faltas e notas e validar período letivo das faltas (feito)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento removendo totalmente do sistema os recursos de editar e eliminar faltas e notas. A partir desta tarefa, faltas e notas devem ser recursos somente de leitura e criação, ou seja, expostos apenas por fluxos `GET` e `POST`. Também garanta que toda falta criada tenha data dentro do período do ano letivo aplicável da academia, seja escolar ou superior. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a código legado, aliases, rotas antigas, wrappers de compatibilidade, fallbacks temporários ou qualquer resquício funcional dos fluxos anteriores.

## Contexto

A regra de produto mudou: faltas e notas não podem mais ser editadas nem eliminadas depois de criadas. O histórico acadêmico deve ser preservado por criação e consulta, não por mutação corretiva ou exclusão operacional. Essa decisão simplifica auditoria, reduz ambiguidades de projeção e evita a manutenção de caminhos antigos que poderiam continuar permitindo alteração de dados sensíveis.

Além disso, faltas devem respeitar rigidamente o calendário letivo da academia. Uma falta só pode ser criada se a data estiver dentro do período do ano letivo vigente correspondente ao tipo letivo da matéria:

- **escolar**: usar o período fixo do ano letivo escolar da academia;
- **superior**: usar o período fixo do ano letivo superior da academia.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Faltas | Remover edição e exclusão | Manter apenas criação e leitura (`POST`/`GET`) |
| Notas | Remover edição e exclusão | Manter apenas criação e leitura (`POST`/`GET`) |
| Validação de data da falta | Obrigatória no fluxo de criação | Rejeitar datas fora do período letivo escolar/superior da academia |
| Documentação | Atualizar integralmente | Remover menções a edição/exclusão, documentar os novos contratos e regras |
| Legado | Proibido | Não manter aliases, rotas antigas, compatibilidade temporária ou código morto |

---

# 1. Remoção total de edição e exclusão de faltas e notas

## Objetivo

Eliminar completamente os recursos que permitem editar ou eliminar faltas e notas. Depois da atualização, o backend deve aceitar apenas:

- criação de faltas;
- listagem/consulta de faltas;
- criação de notas;
- listagem/consulta de notas.

## Escopo obrigatório

### 1.1 Remover rotas e handlers de mutação antiga

Remover por completo qualquer rota, handler, DTO, comando, caso de uso ou wiring relacionado a:

- atualizar/editar falta;
- deletar/eliminar falta;
- atualizar/editar nota;
- deletar/eliminar nota;
- restaurar nota ou falta, se existir;
- qualquer endpoint equivalente usando outro nome, alias ou caminho legado.

A remoção deve ser real, não apenas ocultação na documentação. O roteador não deve registrar essas rotas, e os handlers antigos não devem permanecer como código morto.

### 1.2 Remover eventos e caminhos de escrita obsoletos quando não forem mais necessários

Auditar o modelo de domínio, aggregate, event store, projeções e testes para identificar eventos e applies usados exclusivamente por edição/exclusão de faltas e notas.

Remover o que ficar sem uso funcional, incluindo, quando aplicável:

- eventos de atualização de falta;
- eventos de deleção de falta;
- eventos de atualização de nota;
- eventos de deleção de nota;
- validações exclusivas desses comandos;
- métodos auxiliares que só existiam para esses fluxos.

Se algum evento antigo precisar permanecer apenas para replay de ambientes já migrados, justificar explicitamente no PR e isolar como leitura histórica interna. Porém, para esta tarefa, a orientação preferencial é **não manter suporte legado**. Não criar novos aliases, shims, endpoints de compatibilidade ou caminhos alternativos.

### 1.3 Remover soft delete operacional de faltas e notas ativas

Consultas padrão devem retornar apenas o modelo atual esperado pelo produto. Se existirem colunas/campos como `deleted_at`, `deletado_por`, `motivo_exclusao` ou equivalentes usados apenas para exclusão operacional de faltas/notas, avaliar remoção por migration ou deixar apenas se houver necessidade histórica comprovada.

Não implementar nova funcionalidade de exclusão lógica para substituir exclusão física. A regra agora é: faltas e notas são criadas e consultadas, não editadas nem eliminadas.

### 1.4 Ajustar permissões e contratos de API

Remover permissões, policies, middlewares ou scopes específicos de edição/exclusão de faltas e notas. Garantir que nenhum perfil consiga executar essas ações por rota alternativa.

Atualizar contratos para deixar claro que os recursos aceitos são:

- `POST` para criação;
- `GET` para leitura/listagem.

Endpoints `PUT`, `PATCH` e `DELETE` para faltas/notas não devem existir.

### 1.5 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. rotas antigas de edição/exclusão não estão registradas;
2. não existe permissão funcional para editar falta;
3. não existe permissão funcional para eliminar falta;
4. não existe permissão funcional para editar nota;
5. não existe permissão funcional para eliminar nota;
6. criação de falta continua funcionando;
7. leitura/listagem de faltas continua funcionando;
8. criação de nota continua funcionando;
9. leitura/listagem de notas continua funcionando;
10. replay/projeção continuam consistentes sem depender de novos eventos de edição/exclusão.

---

# 2. Validação obrigatória da data da falta no período letivo

## Objetivo

Garantir que toda falta criada esteja dentro do período do ano letivo da academia correspondente ao tipo letivo da matéria. Essa validação deve ocorrer antes de registrar qualquer evento ou persistir qualquer projeção.

## Regra de negócio

Ao criar uma falta, o backend deve:

1. identificar a academia;
2. identificar o ano letivo vigente da academia;
3. identificar a matéria/disciplina da falta;
4. inferir se a matéria pertence ao fluxo escolar ou superior;
5. carregar o período fixo correto do ano letivo da academia;
6. validar se a data da falta está dentro do intervalo inclusivo `[início, fim]`;
7. rejeitar a criação se a data estiver fora do intervalo.

## Comportamento esperado

- Falta escolar só pode usar data dentro do período escolar da academia.
- Falta superior só pode usar data dentro do período superior da academia.
- A validação deve rejeitar datas anteriores ao início do período.
- A validação deve rejeitar datas posteriores ao fim do período.
- A mensagem de erro deve informar, de forma segura, que a data está fora do período letivo aplicável.
- Não deve haver bypass por payload, parâmetro opcional, endpoint administrativo ou importação em massa.

## Testes obrigatórios

Criar testes cobrindo:

1. criação de falta escolar dentro do período escolar com sucesso;
2. criação de falta escolar antes do início do período escolar rejeitada;
3. criação de falta escolar depois do fim do período escolar rejeitada;
4. criação de falta superior dentro do período superior com sucesso;
5. criação de falta superior antes do início do período superior rejeitada;
6. criação de falta superior depois do fim do período superior rejeitada;
7. matéria inexistente ou não pertencente à academia não permite inferir período e deve ser rejeitada;
8. tentativa de manipular tipo letivo pelo payload não deve sobrescrever a inferência do domínio.

---

# 3. Atualização obrigatória da documentação

## Objetivo

Atualizar toda documentação afetada para refletir que faltas e notas agora são apenas criadas e lidas, e que não existe mais edição/exclusão desses recursos.

## Escopo de documentação

Atualizar, quando existirem:

- documentação de API/OpenAPI/Swagger;
- README técnico;
- documentação de domínio acadêmico;
- documentação de permissões/perfis;
- exemplos de payload;
- coleções de API;
- guias operacionais;
- documentos de tarefas anteriores que ainda sejam usados como referência ativa.

## Regras de documentação

A documentação não deve mencionar rotas antigas como alternativas depreciadas. Não usar termos como “deprecated”, “mantido por compatibilidade” ou “legacy endpoint” para esses fluxos. A remoção deve aparecer como regra vigente: faltas e notas só aceitam criação e leitura.

Também documentar que a data da falta é validada contra o período letivo escolar/superior da academia.

---

# 4. Fora de escopo

- Criar mecanismo de correção retroativa de notas ou faltas.
- Criar restauração de faltas/notas.
- Manter endpoint antigo retornando `410 Gone` como compatibilidade.
- Criar aliases para nomes antigos de rotas.
- Criar flags para reativar edição/exclusão.
- Suportar código legado de clientes antigos.

---

# 5. Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. não existir endpoint `PUT`, `PATCH` ou `DELETE` funcional para faltas e notas;
2. não existir handler/comando ativo para editar ou eliminar faltas e notas;
3. criação e leitura de faltas e notas continuarem funcionando;
4. criação de falta validar data dentro do período letivo escolar/superior da academia;
5. datas fora do período forem rejeitadas antes da persistência/evento;
6. documentação de API e domínio estiver atualizada sem referência a suporte legado;
7. testes automatizados cobrirem remoção de rotas e validação de período;
8. não houver aliases, shims, fallbacks ou código morto dos fluxos removidos;
9. o PR explicar claramente que a mudança remove suporte a edição/exclusão e não mantém compatibilidade com clientes antigos.
