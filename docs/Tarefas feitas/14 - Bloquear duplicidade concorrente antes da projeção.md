---
criado: 2026-07-25 00:00
origem: Solicitação operacional sobre requisições simultâneas aceitas antes da projeção/ledger ficar consultável
status: feito
---

# Bloquear duplicidade concorrente antes da projeção ficar consultável (feito)

## Prompt recomendado para executar a atualização

Implemente um mecanismo transacional e reutilizável de **guarda de unicidade em progresso** para rotas `POST` e `PUT` que criam ou alteram dados com restrição de unicidade funcional. O objetivo é bloquear requisições simultâneas idênticas ou conflitantes quando a primeira ainda está validando, fazendo upload ou gravando evento, e portanto ainda não aparece na projeção usada pelas validações atuais. Antes de codificar, revise `Documentação.md` e confirme a lista de dados únicos abaixo. Ao final, atualize testes, documentação técnica e qualquer contrato afetado. Não crie aliases, wrappers legados, sleeps, validações em memória por processo, nem solução dependente de apenas uma instância da API.

## Contexto

O backend usa event sourcing e valida muitas duplicidades consultando projeções (`projection_*`) e, em alguns casos, também o ledger. Isso funciona quando o primeiro evento já foi gravado e projetado, mas falha em janelas de concorrência: duas requisições simultâneas podem passar pela mesma consulta de existência antes de qualquer uma delas gravar o fato que deveria bloquear a outra.

O caso observado está nas rotas do estudante para solicitação documentada de edição de dados sensíveis: `POST /estudante/solicitacoes-edicao/nome`, `POST /estudante/solicitacoes-edicao/bilhete-identidade`, `POST /estudante/solicitacoes-edicao/bilhete-identidade-encarregado` e `POST /estudante/solicitacoes-edicao/data-nascimento`. A documentação afirma que existe no máximo uma solicitação `pendente` por estudante e campo, e que uma segunda solicitação pendente deve retornar `409`. Porém, se o estudante dispara duas requisições rápidas para o mesmo campo, a segunda pode ser aceita porque a primeira ainda não concluiu upload/gravação/projeção.

O problema não é exclusivo desse fluxo. Sempre que uma rota `POST` ou `PUT` protege dados únicos apenas com leitura de projeção/ledger antes da gravação, existe risco de dupla aceitação em concorrência real, especialmente em rotas com `multipart/form-data`, upload de PDF, jobs ou múltiplos eventos.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Solução | Guarda de unicidade transacional compartilhada no banco | Funciona com múltiplas instâncias da API e sobrevive a concorrência real |
| Escopo inicial obrigatório | Solicitações de edição do estudante e demais rotas documentadas com unicidade funcional | Segunda requisição simultânea retorna `409 Conflict` antes de duplicar eventos |
| Chave de unicidade | `scope` + `key` canônicos por regra de negócio | Mesma entidade/campo/valor gera a mesma guarda em qualquer instância |
| Tempo de vida | Reserva liberada em falha antes do fato persistir; consumida/finalizada quando o evento é gravado | Não deixa bloqueios órfãos permanentes e não substitui constraints definitivas |
| Testes | Concorrência real com goroutines/transações | Prova que duas chamadas simultâneas não conseguem criar duplicidade |

---

# 1. Mapear dados únicos a partir da documentação

## Objetivo

Antes de implementar, confirmar e documentar no PR quais rotas e dados possuem unicidade funcional já definida em `Documentação.md` e precisam de proteção contra concorrência pré-projeção.

## Escopo obrigatório identificado na documentação atual

A implementação deve cobrir, no mínimo, os grupos abaixo.

### 1.1 Solicitações documentadas de edição de dados sensíveis do estudante

Rotas:

- `POST /estudante/solicitacoes-edicao/nome`;
- `POST /estudante/solicitacoes-edicao/bilhete-identidade`;
- `POST /estudante/solicitacoes-edicao/bilhete-identidade-encarregado`;
- `POST /estudante/solicitacoes-edicao/data-nascimento`.

Regra única documentada: deve existir **no máximo uma solicitação `pendente` por `codigo_estudante` e `campo`**. A chave canônica mínima da guarda deve ser equivalente a:

```text
solicitacao_edicao_dado_estudante:pendente:{codigo_estudante}:{campo}
```

Se uma segunda requisição simultânea tentar criar outra solicitação pendente para o mesmo estudante e campo, deve retornar `409 Conflict`, mesmo que a primeira ainda não esteja visível na projeção.

### 1.2 Solicitações de status acadêmico do estudante

Rotas:

- `POST /estudante/solicitacoes-status/interrupcao`;
- `POST /estudante/solicitacoes-status/desvinculacao`;
- `POST /estudante/solicitacoes-status/revinculacao/:codigo_academia`.

Regra única documentada: deve existir **uma única solicitação `pendente` do mesmo tipo para o mesmo estudante na mesma academia**. A chave canônica mínima deve incluir `codigo_estudante`, `codigo_academia` e tipo (`interrupcao`, `desvinculacao`, `revinculacao`).

### 1.3 Cadastro direto e cadastro em lote de estudante

Rotas:

- `POST /academia/estudante/register`;
- `POST /academia/estudantes/batch`;
- `POST /academia/estudantes/batch/async`;
- variações multipart/JSON documentadas do cadastro em massa.

Regras únicas documentadas:

- `codigo_estudante` é gerado pelo backend e deve ser único, já consultando ledger/projeção;
- `bilhete_identidade` do estudante, quando presente, deve ser único entre estudantes;
- em lote com arquivos, `codigo_temporario` deve ser único dentro do request/job.

A guarda concorrente deve proteger principalmente o BI normalizado antes da criação do estudante, porque duas requisições simultâneas com o mesmo BI podem passar na consulta antes da projeção. A geração de `codigo_estudante` deve continuar validando ledger/projeção, mas também não pode depender apenas de projeção atrasada.

### 1.4 Solicitação de matrícula e aprovação de matrícula

Rotas:

- `POST /solicitacao-matricula`;
- `PUT /academia/solicitacao-matricula/:codigo/aprovar`.

Regras relevantes documentadas:

- a validação cadastral comum deve impedir BI do estudante já cadastrado;
- a aprovação cria estudante e deve revalidar duplicidade atual;
- solicitações semelhantes na mesma academia são apenas sinalizadas, não bloqueadas;
- solicitações concorrentes entre academias podem existir enquanto pendentes, mas a aprovação não pode criar estudante duplicado para um BI já criado/aprovado em paralelo.

A guarda não deve transformar o mecanismo de `solicitacoes_semelhantes` em bloqueio na criação da solicitação. Ela deve proteger a criação efetiva de estudante/aprovação com BI único.

### 1.5 Academia, contatos e documentos formais

Rotas:

- `POST /academia/register`;
- `PUT /me/email`;
- `PUT /me/telefone`.

Regras únicas documentadas:

- `email` deve ser único no sistema;
- `telefone` segue normalização estrita e deve respeitar a política de contato único quando já aplicada pelo código;
- `nif` da academia é obrigatório e único, inclusive para academias inativas;
- o código da academia gerado deve continuar único.

### 1.6 Domínios acadêmicos com unicidade por escopo

Rotas a auditar e proteger quando a documentação/código já define unicidade:

- criação/edição de turmas: código da turma único dentro da academia;
- regras de avaliação final: no máximo uma regra raiz ativa por academia, nível e escopo; não pode existir regra ativa com mesmo `type`, `nivel` e escopo sobreposto;
- avaliação final automática/manual: idempotência por estudante, academia, ano letivo, nível interno, ano/período acadêmico atual e `type`;
- pendências de matéria: não pode existir duplicidade aberta para o mesmo estudante, matéria, curso, nível, ano letivo e escopo acadêmico;
- anos letivos e listas históricas: não duplicar `ano_letivo` por academia/tipo e na configuração global.

Quando alguma dessas rotas já estiver protegida por constraint transacional definitiva no banco, registrar isso no PR e não duplicar solução desnecessária. Quando depender apenas de consulta prévia a projeção, adicionar guarda.

---

# 2. Criar mecanismo compartilhado de guarda de unicidade em progresso

## Objetivo

Criar uma infraestrutura única, testável e reutilizável para reservar uma chave de unicidade antes de executar validações caras, upload de documentos ou gravação de eventos.

## Regra de negócio

A guarda deve:

1. receber `scope`, `key`, `aggregate_type`, `aggregate_id` opcional, usuário executor e metadados mínimos;
2. tentar reservar a chave de forma atômica no banco;
3. se a chave já estiver reservada/ativa, retornar erro de conflito mapeado para `409 Conflict` com mensagem clara;
4. liberar a reserva se a requisição falhar antes de persistir o fato de domínio;
5. marcar a reserva como consumida/finalizada quando o evento/fato correspondente for gravado;
6. expirar ou permitir limpeza segura de reservas abandonadas por crash, sem permitir janela curta suficiente para duplicar requisições ainda em andamento.

## Requisitos técnicos obrigatórios

A solução deve ser baseada em PostgreSQL e funcionar em ambiente com múltiplas instâncias da API. Opções aceitáveis:

- tabela `unique_operation_guards` com índice único parcial por (`scope`, `key`) para reservas ativas;
- advisory locks transacionais (`pg_advisory_xact_lock`/`pg_try_advisory_xact_lock`) com chave hash canônica, desde que sejam mantidos durante toda a seção crítica real;
- combinação dos dois mecanismos quando necessário.

Não são aceitáveis como solução principal:

- `sync.Mutex`, mapa em memória ou cache local da instância;
- sleeps/retries cegos;
- depender de ordem de chegada na projeção;
- validar apenas depois do upload, deixando duas requisições fazerem trabalho caro antes de uma falhar;
- criar try/catch ao redor de imports ou padrões fora das diretrizes do projeto.

---

# 3. Integrar a guarda nas rotas de criação/alteração únicas

## Objetivo

Aplicar o mecanismo nas rotas mapeadas na seção 1, priorizando primeiro as solicitações do estudante e depois os demais fluxos com dados únicos.

## Escopo obrigatório

### 3.1 Solicitações de edição do estudante

Adicionar guarda antes de salvar o PDF temporário e antes de gravar `SolicitacaoEdicaoDadoEstudanteCriada`. Para cada campo, a segunda requisição concorrente do mesmo estudante deve falhar com `409` sem criar segundo arquivo temporário permanente e sem gravar segundo evento.

### 3.2 Solicitações de status acadêmico

Adicionar guarda por estudante/academia/tipo de solicitação pendente. A decisão/aprovação/reprovação deve liberar ou encerrar a unicidade funcional conforme o status deixe de ser `pendente`.

### 3.3 BI único de estudante

Adicionar guarda por BI normalizado nos fluxos que criam estudante diretamente ou por aprovação de matrícula. A guarda deve cobrir o intervalo entre a validação de duplicidade e a gravação efetiva de `EstudanteCriadoComVinculo`.

### 3.4 Academia e contatos

Adicionar guarda para `email`, `telefone` se único no comportamento atual, `nif` e códigos gerados quando a validação ainda depender de leitura prévia. Preferir constraints únicas definitivas quando a projeção já for a tabela de verdade operacional do dado.

### 3.5 Regras acadêmicas idempotentes

Auditar as rotas de regras de avaliação, avaliação final e pendências. Onde já houver UPSERT/constraint idempotente, apenas adicionar teste de concorrência. Onde houver apenas consulta prévia, aplicar guarda com chave de escopo correspondente.

---

# 4. Padronizar erro e observabilidade

## Objetivo

Garantir que a falha por concorrência seja clara para cliente, suporte e logs.

## Regras obrigatórias

- retornar `409 Conflict` para reserva já existente;
- usar envelope de erro já padronizado pelo projeto;
- incluir `field`/`code` nos detalhes quando a rota já usa erros estruturados;
- registrar log com `scope`, hash ou chave mascarada quando houver dado sensível, rota e usuário;
- nunca expor BI completo, NIF completo, email completo ou telefone completo em logs de conflito se o padrão atual do projeto já mascarar dados sensíveis.

---

# 5. Testes obrigatórios

## Objetivo

Provar que a correção fecha a janela de concorrência, não apenas a duplicidade sequencial.

## Cenários mínimos

1. duas goroutines criando `POST /estudante/solicitacoes-edicao/nome` para o mesmo estudante/campo: uma retorna sucesso e a outra `409`;
2. repetir o teste para `bilhete-identidade`, `bilhete-identidade-encarregado` e `data-nascimento`;
3. duas requisições para campos diferentes do mesmo estudante podem seguir em paralelo, se a regra de negócio permitir uma pendente por campo;
4. após aprovar/reprovar uma solicitação de edição, uma nova solicitação para o mesmo campo pode ser criada;
5. duas requisições simultâneas de status acadêmico do mesmo tipo/estudante/academia: uma sucesso e outra `409`;
6. duas criações/aprovações simultâneas com o mesmo BI do estudante: no máximo um estudante é criado;
7. falha antes da gravação do evento libera a reserva e permite nova tentativa válida;
8. crash/reserva expirada é tratada por limpeza segura ou regra de expiração testada;
9. testes unitários da normalização das chaves canônicas;
10. testes de regressão garantindo que solicitações semelhantes de matrícula continuam apenas sinalizadas, sem bloqueio indevido.

---

# 6. Atualização obrigatória da documentação

Atualizar `Documentação.md` para declarar que rotas com unicidade funcional possuem proteção contra concorrência pré-projeção e retornam `409 Conflict` quando outra operação equivalente está em andamento. A documentação deve citar explicitamente as rotas de solicitação de edição do estudante, pois elas motivaram esta tarefa.

Se existir OpenAPI/Swagger ou documentação externa de erros, atualizar também os possíveis `409` por `unique_operation_in_progress` ou código equivalente escolhido na implementação.

---

# Fora de escopo

- Trocar a arquitetura de event sourcing ou tornar projeções síncronas globalmente.
- Bloquear solicitações de matrícula semelhantes na criação; a documentação atual define sinalização, não bloqueio.
- Criar mecanismo de identidade nacional única de estudantes.
- Remover validações já existentes em projeção/ledger; a guarda é complementar, não substituta.
- Implementar retries automáticos no cliente.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. existir mecanismo compartilhado de guarda de unicidade em progresso, transacional e seguro para múltiplas instâncias;
2. as quatro rotas `POST /estudante/solicitacoes-edicao/*` bloquearem duplicidade concorrente por estudante/campo com `409`;
3. solicitações de status acadêmico bloquearem duplicidade concorrente por estudante/academia/tipo;
4. criação direta/aprovação de matrícula não criarem dois estudantes com o mesmo BI em concorrência;
5. rotas de academia/contato/NIF/código e domínios acadêmicos documentados tiverem sido protegidos ou justificados no PR por já possuírem constraint transacional suficiente;
6. reservas sejam liberadas em falha antes do evento e finalizadas quando o fato único é persistido;
7. logs e erros sigam o padrão do projeto, sem vazar dados sensíveis;
8. testes automatizados de concorrência real cubram os cenários da seção 5;
9. `Documentação.md` e documentação de erros estejam atualizadas;
10. o PR liste todas as chaves de unicidade implementadas e as rotas auditadas.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Bloquear duplicidade concorrente antes da projeção ficar consultável (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
