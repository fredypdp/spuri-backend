---
criado: 2026-07-08 00:00
origem: solicitação do usuário
status: pronto_para_implementacao
---

# Restringir pendências ao ensino superior e ativar matérias superiores por padrão

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que os campos `pendencia_permitida` e `pendencia_nivel_conclusao` sejam exclusivos de matérias do ensino superior. O contrato de criação/atualização de matérias superiores deve manter `pendencia_permitida` no request, mas o backend já deve inferir o valor efetivo como `true` quando o campo não vier preenchido. Matérias do ensino superior também devem ser criadas com status `ativada` por padrão. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a código legado, aliases, wrappers de compatibilidade, fallbacks temporários ou regras paralelas que permitam configurar pendência em matérias escolares.

## Contexto

A regra de produto mudou para separar de forma rígida o comportamento de pendências acadêmicas entre ensino escolar e ensino superior. A pendência passa a ser um recurso exclusivo do fluxo superior, porque esse fluxo permite progressão semestral com dependências de disciplinas, enquanto matérias escolares não devem expor nem persistir configuração de pendência.

Além disso, a criação de matérias do ensino superior deve refletir o comportamento padrão esperado pela academia:

- matérias superiores mantêm `pendencia_permitida` no request;
- matérias superiores aceitam pendência por padrão, com inferência backend para `true` quando o campo não vier preenchido;
- matérias superiores são criadas com status `ativada` por padrão;
- matérias escolares não devem aceitar, expor ou persistir configuração de pendência.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| `pendencia_permitida` | Exclusivo do ensino superior | Rejeitar/remover uso em matérias escolares |
| `pendencia_nivel_conclusao` | Exclusivo do ensino superior | Rejeitar/remover uso em matérias escolares |
| Request de matéria superior | Mantém `pendencia_permitida` | O campo deve fazer parte do request de matérias superiores |
| Matéria superior nova | Aceita pendência por padrão | Backend infere `pendencia_permitida = true` quando o campo não vier preenchido, salvo regra explícita válida em contrário |
| Status de matéria superior nova | Ativada por padrão | Criar matéria superior com status `ativada` quando o payload não informar status |
| Documentação | Atualizar integralmente | Contratos e guias devem diferenciar claramente escolar e superior |
| Legado | Proibido | Não manter aliases, compatibilidade temporária ou fallback que aceite pendência em matéria escolar |

---

# 1. Exclusividade dos campos de pendência para ensino superior

## Objetivo

Garantir que `pendencia_permitida` e `pendencia_nivel_conclusao` existam funcionalmente apenas para matérias do ensino superior. Matérias escolares não devem aceitar esses campos em payloads, não devem expor esses campos em respostas como configuração válida e não devem depender deles em regras de domínio.

## Escopo obrigatório

### 1.1 Ajustar validação de entrada

Atualizar DTOs, schemas, validators, commands e casos de uso para impedir o uso de `pendencia_permitida` e `pendencia_nivel_conclusao` em matérias escolares.

Quando o backend receber esses campos para uma matéria escolar, o comportamento deve ser explícito e seguro:

- rejeitar a requisição com erro de validação claro; ou
- remover os campos do contrato escolar se houver separação de DTOs por tipo letivo.

Não permitir que o backend ignore silenciosamente valores enviados para matéria escolar se isso puder gerar ambiguidade para clientes.

### 1.2 Ajustar modelo de domínio e persistência

Auditar entidades, aggregates, migrations, repositories, projections e serializers para garantir que a configuração de pendência seja aplicada apenas a matérias superiores.

Se os campos permanecerem fisicamente em tabela compartilhada por necessidade estrutural, garantir que:

- matérias escolares não sejam criadas com valores funcionais nesses campos;
- consultas e respostas escolares não apresentem pendência como recurso configurável;
- regras de negócio nunca usem esses campos para matérias escolares;
- constraints, validações de domínio ou testes impeçam estados inválidos.

### 1.3 Remover caminhos alternativos de configuração escolar

Remover ou ajustar qualquer endpoint, importação, seed, factory, script administrativo ou rotina interna que permita configurar pendência para matéria escolar.

Não criar flags, aliases, modo legado, bypass administrativo ou compatibilidade temporária para manter pendência em matérias escolares.

### 1.4 Ajustar respostas e contratos de API

Atualizar OpenAPI/Swagger e serializers para deixar claro que:

- `pendencia_permitida` deve fazer parte do request de matérias superiores;
- campos de pendência pertencem ao contrato de matérias superiores;
- contratos de matérias escolares não aceitam esses campos;
- respostas escolares não devem sugerir que pendência é configurável;
- respostas superiores devem refletir os valores efetivos de pendência.

### 1.5 Atualizar testes

Adicionar ou ajustar testes cobrindo:

1. criação de matéria escolar com `pendencia_permitida` deve ser rejeitada ou impossível pelo contrato;
2. criação de matéria escolar com `pendencia_nivel_conclusao` deve ser rejeitada ou impossível pelo contrato;
3. atualização de matéria escolar, se existir, não deve aceitar campos de pendência;
4. importações, seeds ou factories não devem criar matéria escolar com pendência funcional;
5. matéria superior continua aceitando `pendencia_permitida`;
6. matéria superior continua aceitando `pendencia_nivel_conclusao` quando aplicável;
7. respostas de matérias escolares não expõem pendência como configuração válida;
8. respostas de matérias superiores expõem pendência conforme o contrato vigente.

---

# 2. Matérias do ensino superior devem aceitar pendência por padrão

## Objetivo

Garantir que toda matéria superior criada sem configuração explícita de pendência seja criada aceitando pendência por padrão.

## Regra de negócio

Ao criar uma matéria do ensino superior, o backend deve:

1. identificar que a matéria pertence ao fluxo superior;
2. manter `pendencia_permitida` como campo do request de matérias superiores;
3. inferir no backend `pendencia_permitida = true` quando o campo não vier preenchido;
4. validar `pendencia_nivel_conclusao` somente dentro das regras permitidas para ensino superior;
5. persistir e retornar o valor efetivo aplicado;
6. usar esse valor efetivo nas regras acadêmicas de progressão, pendência e conclusão.

## Comportamento esperado

- `pendencia_permitida` deve existir no request de criação/atualização de matéria superior.
- Matéria superior criada sem valor preenchido para `pendencia_permitida` deve ficar com `pendencia_permitida = true`, inferido pelo backend.
- Matéria superior criada com `pendencia_permitida = true` deve manter `true`.
- Matéria superior criada com `pendencia_permitida = false`, se o contrato permitir desativação explícita, deve manter `false`.
- `pendencia_nivel_conclusao` só deve ser aceito e validado para matérias superiores.
- Matérias escolares não devem herdar esse padrão.

## Testes obrigatórios

Criar testes cobrindo:

1. contrato de criação/atualização de matéria superior contém `pendencia_permitida` no request;
2. criação de matéria superior sem valor preenchido para `pendencia_permitida` aplica `true` por inferência do backend;
3. criação de matéria superior com `pendencia_permitida = true` mantém `true`;
4. criação de matéria superior com `pendencia_permitida = false`, se permitido, mantém `false`;
5. criação de matéria superior com `pendencia_nivel_conclusao` válido persiste o valor;
6. criação de matéria superior com `pendencia_nivel_conclusao` inválido é rejeitada;
7. criação de matéria escolar não aplica pendência por padrão;
8. regras de progressão superior usam o padrão efetivo de pendência da matéria.

---

# 3. Matérias do ensino superior devem ser criadas ativadas por padrão

## Objetivo

Garantir que matérias do ensino superior sejam criadas com status `ativada` por padrão quando o payload não informar status explícito válido.

## Regra de negócio

Ao criar uma matéria do ensino superior, o backend deve:

1. identificar que a matéria pertence ao fluxo superior;
2. aplicar status `ativada` quando o payload não informar status;
3. validar status explícito apenas contra os valores permitidos pelo domínio;
4. persistir e retornar o status efetivo aplicado;
5. garantir que a matéria recém-criada esteja disponível para os fluxos acadêmicos esperados.

## Comportamento esperado

- Matéria superior criada sem status deve ficar com status `ativada`.
- Matéria superior criada explicitamente com status `ativada` deve manter `ativada`.
- Se o contrato permitir criação com outro status, esse valor deve ser validado pelas regras existentes.
- O padrão de ativação deve ser aplicado apenas ao ensino superior, sem alterar indevidamente o comportamento escolar.

## Testes obrigatórios

Criar testes cobrindo:

1. criação de matéria superior sem status aplica `ativada` por padrão;
2. criação de matéria superior com status `ativada` mantém `ativada`;
3. criação de matéria superior com status inválido é rejeitada;
4. criação de matéria escolar mantém seu comportamento de status vigente;
5. listagem/consulta de matéria superior recém-criada retorna status `ativada`;
6. fluxos que dependem de matérias ativas passam a enxergar a matéria superior criada por padrão.

---

# 4. Atualização obrigatória da documentação

## Objetivo

Atualizar toda documentação afetada para refletir a separação entre matérias escolares e superiores, especialmente nos contratos de pendência e no comportamento padrão de criação de matérias superiores.

## Escopo de documentação

Atualizar, quando existirem:

- documentação de API/OpenAPI/Swagger;
- README técnico;
- documentação de domínio acadêmico;
- documentação de cursos, matérias, progressão e pendências;
- exemplos de payload;
- coleções de API;
- guias operacionais;
- documentos de tarefas anteriores usados como referência ativa.

## Regras de documentação

A documentação deve declarar explicitamente que `pendencia_permitida` e `pendencia_nivel_conclusao` são exclusivos do ensino superior. Não documentar esses campos como depreciados para matérias escolares; eles não devem fazer parte do contrato escolar vigente.

Também documentar que `pendencia_permitida` deve estar no request de matéria superior e que, quando vier sem valor preenchido, o backend deve inferir:

- `pendencia_permitida = true`;
- status `ativada`.

---

# 5. Fora de escopo

- Criar pendência para ensino escolar.
- Criar compatibilidade para payload escolar contendo campos de pendência.
- Criar aliases para nomes antigos de campos.
- Criar flags para reativar pendência em matérias escolares.
- Alterar regras de conclusão escolar que não dependam da separação de pendências.
- Criar migração de dados histórica sem necessidade identificada durante a implementação.

---

# 6. Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `pendencia_permitida` for funcionalmente exclusivo de matérias superiores;
2. `pendencia_nivel_conclusao` for funcionalmente exclusivo de matérias superiores;
3. matéria escolar não aceitar campos de pendência em criação, atualização, importação ou fluxo administrativo;
4. `pendencia_permitida` estar presente no request de matérias superiores;
5. matéria superior criada sem valor preenchido para `pendencia_permitida` ficar com `pendencia_permitida = true` por inferência do backend;
6. matéria superior criada sem status ficar com status `ativada`;
7. respostas e documentação diferenciarem claramente contratos escolares e superiores;
8. testes automatizados cobrirem exclusividade de pendência, presença de `pendencia_permitida` no request superior, inferência backend para `true` e status padrão superior;
9. não houver aliases, shims, fallbacks, código morto ou compatibilidade temporária para pendência escolar;
10. o PR explicar claramente que pendência é exclusiva do ensino superior e que matérias superiores agora nascem com pendência permitida e status ativado por padrão.
