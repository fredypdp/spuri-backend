---
criado: 2026-07-20 00:00
origem: Lista de tarefas.md
status: pendente
---

# Revisar e ampliar eventos de progressão e status acadêmico do estudante (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento removendo as rotas públicas de matrícula de etapa (`POST /academia/estudante/:codigo/matricula/fundamental`, `POST /academia/estudante/:codigo/matricula/medio`, `POST /academia/estudante/:codigo/matricula/superior`) e seus eventos/consequências, substituindo as rotas separadas de interrupção/trancamento por um fluxo obrigatório de solicitação do estudante e aprovação da academia em `POST /academia/estudante/:codigo/interromper/percurso-academico`, alterando a desvinculação para definir `status = "inativo"` em vez de `arquivado`, e adicionando mecanismo de solicitação -> aprovação também para desvinculação e revinculação. Ao final, atualize testes, documentação técnica e qualquer documentação afetada. Não criar suporte a código legado, aliases, wrappers de compatibilidade, migração de registros antigos ou fallbacks para eventos removidos, pois o banco de dados está vazio.

## Contexto

A seção "Endpoints de acontecimentos que alteram status do estudante" foi criada com rotas legadas que tratavam matrícula de etapa como acontecimento separado (`matricula/fundamental`, `matricula/medio`, `matricula/superior`) mesmo quando a criação do estudante e do vínculo já registram a matrícula inicial no domínio. Essa duplicidade aumenta a complexidade do event sourcing, cria mais caminhos para alterar o mesmo status e dificulta o raciocínio sobre o estado real do estudante.

Além disso, a interrupção de percurso acadêmico e as operações de desvinculação/revinculação são sensíveis demais para serem executadas unilateralmente pela academia. A nova regra de produto exige participação explícita do estudante: o estudante solicita, a academia aprova, e somente a aprovação emite o evento que altera o status.

Esta tarefa substitui a modelagem antiga por uma modelagem mais simples e auditável:

1. matrícula inicial é consequência da criação/vínculo do estudante, não de endpoints posteriores de `matricula/*`;
2. interrupção de percurso acadêmico passa por solicitação do estudante e aprovação da academia;
3. desvinculação e revinculação também passam por solicitação do estudante e aprovação da academia;
4. `status = "arquivado"` deixa de existir; estudante desvinculado fica `status = "inativo"`;
5. não há compatibilidade com eventos/rotas antigas, porque não há dados legados a preservar.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Rotas `matricula/*` | Remover rotas, handlers, DTOs, validações, eventos e documentação | Matrícula inicial fica representada por `EstudanteCriadoComVinculo`/aprovação da solicitação/cadastro direto |
| Interrupção/trancamento por nível | Remover rotas diretas antigas | Uma única rota de solicitação/execução aprovada cobre fundamental, médio e superior |
| Evento superior | Substituir `SuperiorTrancado` por `SuperiorInterrompido` | Nomenclatura unificada de interrupção de percurso acadêmico |
| Solicitação -> aprovação | Obrigatória para interrupção, desvinculação e revinculação | Academia não executa essas ações sozinha |
| Status geral | Remover `arquivado`; usar `inativo` | `EstudanteDesvinculadoDaAcademia` define `status = "inativo"` |
| Revinculação fundamental | Bloquear se o estudante progrediu de ano em outra academia | Evitar retorno para ano acadêmico defasado/inconsistente |
| Legado | Não suportar rotas/eventos antigos | Banco vazio; implementação pode ser limpa e sem compatibilidade retroativa |

---

# 1. Remover rotas de matrícula de etapa e seus eventos

## Objetivo

Remover a modelagem em que a matrícula de uma etapa acadêmica é executada por rotas públicas separadas após a criação do estudante.

## Rotas a remover

Remover completamente:

- `POST /academia/estudante/:codigo/matricula/fundamental`;
- `POST /academia/estudante/:codigo/matricula/medio`;
- `POST /academia/estudante/:codigo/matricula/superior`.

## Eventos e consequências a remover

Remover, junto com os endpoints, todos os elementos que existam exclusivamente para suportá-los, incluindo, quando aplicável:

- eventos:
  - `MatriculaFundamentalEfetivada`;
  - `MatriculaMedioEfetivada`;
  - `MatriculaSuperiorEfetivada`;
- handlers HTTP;
- DTOs/request structs;
- métodos de aggregate/use case específicos;
- whitelists de eventos;
- projections específicas;
- testes específicos desses endpoints;
- documentação de API;
- exemplos de request/response;
- qualquer rota registrada no router.

## Regra de negócio substituta

A matrícula inicial do estudante deve estar representada no momento em que o estudante é criado/vinculado à academia, por exemplo:

- aprovação de solicitação de matrícula;
- cadastro direto feito pela academia;
- evento `EstudanteCriadoComVinculo` ou evento equivalente de criação/vínculo atualmente usado pelo domínio.

Esse evento de criação/vínculo deve ser suficiente para responder:

1. em qual academia o estudante entrou;
2. em qual nível/ano/curso/período iniciou;
3. qual status inicial foi definido;
4. qual usuário/fluxo originou a criação.

## Médio: validação por modelo do curso

Ao remover `matricula/medio`, garantir que qualquer validação de ingresso inicial ou revinculação no médio respeite os anos derivados do modelo do curso:

| Modelo do curso médio | Anos permitidos |
| --- | --- |
| `liceu` | `1_ano_medio`, `2_ano_medio`, `3_ano_medio` |
| `tecnico` | `1_ano_medio`, `2_ano_medio`, `3_ano_medio`, `4_ano_medio` |

O backend não deve aceitar `ano_escolar_medio` incompatível com o `modelo` do curso médio.

## Testes obrigatórios

1. as três rotas `matricula/*` deixam de existir e retornam `404` ou o comportamento padrão de rota inexistente;
2. os eventos `MatriculaFundamentalEfetivada`, `MatriculaMedioEfetivada` e `MatriculaSuperiorEfetivada` não são mais aceitos/criados em nenhum fluxo público;
3. cadastro direto continua criando estudante/vínculo com status inicial correto;
4. aprovação de solicitação de matrícula continua criando estudante/vínculo com status inicial correto;
5. ingresso/cadastro no médio rejeita ano incompatível com o modelo do curso;
6. não existe fallback, alias ou wrapper que reative as rotas removidas.

---

# 2. Substituir interrupção/trancamento direto por solicitação de interrupção de percurso acadêmico

## Objetivo

Remover as rotas diretas por nível que permitiam à academia interromper/trancar o percurso sozinha e substituí-las por um mecanismo obrigatório de solicitação do estudante e aprovação pela academia.

## Rotas antigas a remover

Remover completamente:

- `POST /academia/estudante/:codigo/interrupcao/fundamental`;
- `POST /academia/estudante/:codigo/interrupcao/medio`;
- `POST /academia/estudante/:codigo/trancamento/superior`.

## Nova rota de solicitação/execução

Criar o fluxo público/autenticado adequado para o estudante solicitar a interrupção do próprio percurso acadêmico e para a academia aprovar ou reprovar a solicitação.

A rota de referência do escopo é:

```text
POST /academia/estudante/:codigo/interromper/percurso-academico
```

Essa rota representa a ação de interrupção aprovada no contexto da academia, mas **não pode permitir execução unilateral pela academia**. A implementação deve criar um mecanismo completo de solicitação -> decisão, com contratos claros para:

1. estudante solicitar interrupção do percurso acadêmico;
2. academia listar/consultar solicitações pendentes;
3. academia aprovar a solicitação;
4. academia reprovar a solicitação com motivo;
5. somente a aprovação emitir o evento de interrupção do percurso.

A equipe pode ajustar a divisão exata de endpoints desde que preserve a semântica obrigatória: estudante solicita, academia decide, evento só nasce na aprovação.

## Regra de decisão do evento

Na aprovação, o backend deve identificar a etapa acadêmica em andamento do estudante e emitir o evento adequado:

| Etapa atual em andamento | Evento a emitir | Status resultante |
| --- | --- | --- |
| Fundamental | `FundamentalInterrompido` | `status_escolar_fundamental = "inativo"` |
| Médio | `MedioInterrompido` | `status_escolar_medio = "inativo"` |
| Superior | `SuperiorInterrompido` | `status_superior = "inativo"` |

`SuperiorTrancado` deve ser removido/renomeado para `SuperiorInterrompido`. Não manter alias nem compatibilidade com `SuperiorTrancado`.

## Regras de negócio mantidas/adaptadas

1. Só é permitido interromper uma etapa que esteja `em_andamento`.
2. Deve existir exatamente uma etapa acadêmica interrompível em andamento, ou o backend deve rejeitar com erro claro quando não conseguir determinar a etapa de forma inequívoca.
3. `motivo` é obrigatório na solicitação do estudante e não pode ser vazio após `trim`.
4. A aprovação da academia não pode alterar o motivo original do estudante; se houver observação da academia, ela deve ser campo separado.
5. A interrupção não apaga notas, faltas, avaliações, turmas, documentos, curso, ano, semestre ou histórico.
6. Solicitação decidida não volta para pendente.
7. Não pode existir mais de uma solicitação pendente de interrupção para o mesmo estudante na mesma academia.
8. A academia só pode decidir solicitações de estudantes vinculados a ela.

## Status de solicitação

Usar conjunto fechado de status, por exemplo:

```text
pendente
aprovada
reprovada
cancelada
```

`cancelada` só deve existir se houver caso funcional definido, por exemplo cancelamento pelo próprio estudante antes da decisão. Caso contrário, não implementar `cancelada`.

## Auditoria

A solicitação e a decisão devem ser auditáveis. O evento final de interrupção deve registrar, no mínimo:

- estudante;
- academia;
- tipo de ensino identificado;
- motivo informado pelo estudante;
- usuário/academia que aprovou;
- data/hora da solicitação;
- data/hora da aprovação;
- IP da aprovação quando disponível;
- referência da solicitação que originou o evento.

## Testes obrigatórios

1. estudante com fundamental `em_andamento` solicita interrupção; academia aprova; evento `FundamentalInterrompido` é gravado;
2. estudante com médio `em_andamento` solicita interrupção; academia aprova; evento `MedioInterrompido` é gravado;
3. estudante com superior `em_andamento` solicita interrupção; academia aprova; evento `SuperiorInterrompido` é gravado;
4. rota antiga `trancamento/superior` não existe mais;
5. evento `SuperiorTrancado` não é mais emitido nem aceito;
6. academia tentando executar interrupção sem solicitação prévia é rejeitada;
7. solicitação com motivo vazio é rejeitada;
8. solicitação duplicada pendente para o mesmo estudante é rejeitada;
9. aprovação por academia que não é dona do estudante é rejeitada;
10. reprovação da solicitação não altera nenhum status acadêmico.

---

# 3. Alterar desvinculação: remover `arquivado` e usar `inativo`

## Objetivo

Remover o status geral `arquivado` do domínio do estudante. A desvinculação da academia deve definir o estudante como `inativo`, preservando histórico.

## Regra de negócio

O evento `EstudanteDesvinculadoDaAcademia` deve passar a definir:

```text
status = "inativo"
```

em vez de:

```text
status = "arquivado"
```

## Escopo obrigatório

Remover `arquivado` de todos os locais onde ainda apareça como status funcional do estudante, incluindo, quando aplicável:

- enums/constantes;
- validações;
- handlers;
- aggregates;
- projections;
- rebuilds;
- responses;
- filtros;
- testes;
- documentação;
- mensagens de erro.

Não criar mapeamento legado `arquivado -> inativo`, pois não há banco com dados antigos a migrar.

## Regras mantidas/adaptadas

1. Desvinculação não apaga histórico.
2. Desvinculação não remove notas, faltas, avaliações, documentos, turmas ou eventos.
3. Apenas estudante atualmente vinculado/ativo na academia pode ser desvinculado.
4. O evento deve continuar registrando o nível acadêmico atual calculado pelo backend no momento da saída.
5. Operações que antes verificavam `arquivado` devem ser adaptadas para `inativo` quando fizer sentido, sem preservar compatibilidade com `arquivado`.

## Testes obrigatórios

1. `EstudanteDesvinculadoDaAcademia` define `status = "inativo"`;
2. nenhum fluxo público retorna `status = "arquivado"`;
3. rebuild de projeções não produz `arquivado`;
4. filtros/consultas aceitam o novo comportamento e não documentam `arquivado`;
5. histórico acadêmico permanece preservado após desvinculação.

---

# 4. Criar solicitação -> aprovação para desvinculação

## Objetivo

A rota `POST /academia/estudante/:codigo/desvincular` não deve mais permitir execução unilateral pela academia. A desvinculação deve ser solicitada pelo estudante e aprovada pela academia.

## Regra de negócio

Criar mecanismo de solicitação -> decisão para desvinculação:

1. estudante solicita desvinculação informando `motivo` obrigatório;
2. academia consulta/lista solicitações pendentes;
3. academia aprova ou reprova;
4. somente a aprovação emite `EstudanteDesvinculadoDaAcademia`;
5. o evento define `status = "inativo"`.

A rota `POST /academia/estudante/:codigo/desvincular` pode ser mantida como rota de aprovação/execução **apenas se** ela exigir referência a uma solicitação pendente válida do estudante. Ela não pode criar o evento sem solicitação prévia.

## Regras mantidas/adaptadas

1. `motivo` é obrigatório na solicitação do estudante e não pode ser vazio.
2. A academia não pode alterar o motivo informado pelo estudante; observação da academia deve ser campo separado.
3. Apenas academia dona do vínculo pode aprovar/reprovar.
4. Solicitação decidida é terminal.
5. Não pode existir mais de uma solicitação pendente de desvinculação para o mesmo estudante na mesma academia.
6. Aprovação registra no evento o nível acadêmico atual calculado pelo backend.

## Testes obrigatórios

1. estudante solicita desvinculação; academia aprova; `EstudanteDesvinculadoDaAcademia` é gravado com `status = "inativo"`;
2. academia tenta desvincular sem solicitação pendente: rejeitado;
3. estudante solicita com motivo vazio: rejeitado;
4. solicitação duplicada pendente: rejeitada;
5. reprovação não altera status;
6. aprovação por academia diferente da dona do vínculo: rejeitada;
7. evento contém referência da solicitação aprovada.

---

# 5. Criar solicitação -> aprovação para revinculação

## Objetivo

A rota `POST /academia/estudante/:codigo/revincular` também deve passar a exigir solicitação do estudante e aprovação da academia.

## Regra de negócio

Criar mecanismo de solicitação -> decisão para revinculação:

1. estudante `inativo` solicita revinculação à academia;
2. estudante informa o `tipo_ensino` de retorno quando necessário;
3. para médio/superior, estudante pode informar curso pretendido quando houver mudança real de curso;
4. academia aprova ou reprova;
5. somente a aprovação emite `EstudanteReintegrado` ou o evento equivalente de revinculação;
6. aprovação define `status = "ativo"` e reativa a etapa indicada/derivada como `em_andamento`, respeitando as regras de negócio mantidas.

A rota `POST /academia/estudante/:codigo/revincular` pode ser mantida como rota de aprovação/execução **apenas se** exigir referência a uma solicitação pendente válida do estudante. Ela não pode reintegrar sem solicitação prévia.

## Regras mantidas/adaptadas

1. Apenas estudante com `status = "inativo"` por desvinculação pode solicitar/ser revinculado.
2. `tipo_ensino` continua restrito a `fundamental`, `medio` ou `superior` quando precisar ser enviado.
3. No médio, curso informado precisa existir, estar ativo, pertencer à academia e ser do tipo `medio`.
4. No superior, curso informado precisa existir, estar ativo, pertencer à academia e ser do tipo `superior`.
5. Se curso médio/superior for omitido, o backend reutiliza o curso anterior quando isso for válido; se não houver curso anterior, rejeita.
6. Se curso informado for diferente do anterior, aplicar a regra de reinício/progressão definida para o domínio, desde que isso não contradiga histórico acadêmico já consolidado.
7. Revinculação não apaga histórico.
8. Solicitação decidida é terminal.
9. Não pode existir mais de uma solicitação pendente de revinculação para o mesmo estudante na mesma academia.

## Regra especial para fundamental

Para `tipo_ensino = "fundamental"`, a revinculação só pode ser aprovada se o estudante ainda estiver no mesmo ano acadêmico fundamental em que estava quando foi desvinculado daquela academia.

O sistema deve conseguir identificar se, após a desvinculação, o estudante continuou em outra academia e progrediu de ano. Se houver evidência de progressão externa no Spuri, a revinculação fundamental para o ano antigo deve ser bloqueada.

### Dados mínimos para validar a regra

A implementação deve preservar/consultar informação suficiente para comparar:

1. ano fundamental no momento da desvinculação;
2. academia de origem da desvinculação;
3. histórico posterior do estudante em outras academias;
4. ano fundamental atual/mais recente do estudante no sistema;
5. eventos de avaliação final/progressão que tenham ocorrido após a desvinculação.

### Resultado esperado

- Se o estudante saiu no `4_ano_fundamental` e não progrediu em outra academia, a revinculação no `4_ano_fundamental` pode ser aprovada.
- Se o estudante saiu no `4_ano_fundamental`, entrou em outra academia e progrediu para `5_ano_fundamental`, a revinculação para o antigo `4_ano_fundamental` deve ser rejeitada.

## Testes obrigatórios

1. estudante `inativo` solicita revinculação; academia aprova; status vira `ativo`;
2. academia tenta revincular sem solicitação pendente: rejeitado;
3. solicitação duplicada pendente: rejeitada;
4. reprovação não altera status;
5. revinculação fundamental no mesmo ano da desvinculação: sucesso;
6. revinculação fundamental após progressão em outra academia: rejeitada;
7. revinculação médio reutilizando curso anterior válido: sucesso;
8. revinculação médio com curso incompatível/inexistente: rejeitada;
9. revinculação superior reutilizando curso anterior válido: sucesso;
10. revinculação superior com curso incompatível/inexistente: rejeitada;
11. evento final contém referência da solicitação aprovada.

---

# 6. Atualização obrigatória da documentação

Atualizar `Documentação.md`, seção "Endpoints de acontecimentos que alteram status do estudante", para refletir a nova modelagem:

1. remover completamente as rotas `matricula/fundamental`, `matricula/medio` e `matricula/superior`;
2. remover completamente as rotas `interrupcao/fundamental`, `interrupcao/medio` e `trancamento/superior`;
3. documentar o novo fluxo de solicitação -> aprovação para interrupção de percurso acadêmico;
4. documentar `POST /academia/estudante/:codigo/interromper/percurso-academico` conforme a decisão final de contrato;
5. documentar `FundamentalInterrompido`, `MedioInterrompido` e `SuperiorInterrompido`;
6. remover `SuperiorTrancado`;
7. documentar que `EstudanteDesvinculadoDaAcademia` define `status = "inativo"`;
8. remover qualquer menção a `status = "arquivado"` como status funcional atual;
9. documentar que desvinculação e revinculação exigem solicitação do estudante e aprovação da academia;
10. documentar a regra especial de revinculação fundamental no mesmo ano acadêmico da desvinculação.

## Atualizar documentação técnica interna

Atualizar também, quando aplicável:

- OpenAPI/Swagger;
- exemplos de cURL;
- README ou docs de eventos;
- documentação de rebuild/event sourcing;
- documentação de status do estudante;
- arquivos de tarefas feitas que ainda sejam usados como referência técnica, se estiverem contradizendo o novo contrato.

---

# 7. Migração e remoção de legado

## Regra obrigatória

Não criar suporte a código ou registro legado.

O banco de dados está vazio, portanto a implementação deve remover o legado de forma limpa:

- sem migração de dados antigos;
- sem tradução `arquivado -> inativo` para registros existentes;
- sem aceitar eventos antigos no append de novos eventos;
- sem replay compatível com `Matricula*Efetivada` ou `SuperiorTrancado`;
- sem endpoints ocultos/depreciados;
- sem aliases temporários;
- sem feature flag para reativar comportamento antigo.

Se existirem migrations necessárias, elas devem apenas refletir o novo schema/contrato, não preservar dados antigos.

---

# Fora de escopo

- Criar UI/frontend para as solicitações.
- Implementar comunicação por email/SMS/push das solicitações.
- Alterar cálculo de avaliação final, notas ou faltas, exceto quando necessário para validar progressão na revinculação fundamental.
- Criar fluxo de reconhecimento externo de conclusão acadêmica nesta tarefa, salvo se estritamente necessário para manter coerência da revinculação.
- Manter qualquer compatibilidade com rotas/eventos removidos.
- Criar endpoints administrativos para executar interrupção/desvinculação/revinculação sem solicitação do estudante.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `POST /academia/estudante/:codigo/matricula/fundamental` não existir mais;
2. `POST /academia/estudante/:codigo/matricula/medio` não existir mais;
3. `POST /academia/estudante/:codigo/matricula/superior` não existir mais;
4. `MatriculaFundamentalEfetivada`, `MatriculaMedioEfetivada` e `MatriculaSuperiorEfetivada` forem removidos do fluxo público e do pipeline de novos eventos;
5. `POST /academia/estudante/:codigo/interrupcao/fundamental` não existir mais;
6. `POST /academia/estudante/:codigo/interrupcao/medio` não existir mais;
7. `POST /academia/estudante/:codigo/trancamento/superior` não existir mais;
8. existir fluxo de solicitação do estudante e aprovação da academia para interrupção de percurso acadêmico;
9. aprovação de interrupção emitir `FundamentalInterrompido`, `MedioInterrompido` ou `SuperiorInterrompido`, conforme a etapa real do estudante;
10. `SuperiorTrancado` não ser mais emitido nem aceito;
11. `EstudanteDesvinculadoDaAcademia` definir `status = "inativo"`;
12. `status = "arquivado"` não existir mais como status funcional do estudante;
13. desvinculação exigir solicitação do estudante e aprovação da academia;
14. revinculação exigir solicitação do estudante e aprovação da academia;
15. academia não conseguir executar interrupção, desvinculação ou revinculação sozinha;
16. revinculação fundamental ser bloqueada quando o estudante tiver progredido de ano em outra academia após a desvinculação;
17. regras de negócio mantidas das rotas atualizadas continuarem válidas, adaptadas ao novo fluxo de solicitação -> aprovação;
18. `Documentação.md` e documentação técnica refletirem integralmente o novo contrato;
19. testes automatizados cobrirem os cenários obrigatórios desta tarefa;
20. o PR explicar explicitamente que não há suporte legado porque o banco está vazio.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Revisar e ampliar eventos de progressão e status acadêmico do estudante (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
