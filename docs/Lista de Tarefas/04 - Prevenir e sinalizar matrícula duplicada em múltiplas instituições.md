---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Prevenir e sinalizar matrícula duplicada em múltiplas instituições (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento garantindo que `POST /solicitacao-matricula` (1) identifique, dentro da mesma academia, solicitações semelhantes já existentes com base em nome, data de nascimento, gênero e Bilhete de Identidade do estudante ou do responsável, registrando os identificadores dessas solicitações semelhantes num novo campo da solicitação recém-criada; e (2) permita que o mesmo estudante solicite matrícula em academias diferentes, cancelando automaticamente todas as demais solicitações pendentes dele — em qualquer academia — assim que uma delas for aprovada, mas apenas quando a solicitação aprovada tiver `bilhete_identidade` do próprio estudante preenchido. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou bloqueios automáticos não solicitados nesta tarefa.

## Contexto

Hoje `SolicitacaoMatricula` não possui nenhum mecanismo de correlação entre solicitações diferentes, mesmo quando elas claramente pertencem à mesma pessoa. Isso abre espaço para dois problemas distintos:

1. **Duplicidade dentro da mesma academia**: nada impede que a mesma pessoa (ou alguém em nome dela) crie duas ou mais solicitações de matrícula pendentes na mesma academia. A academia analisa cada solicitação isoladamente, sem visibilidade de que já existe outra semelhante em aberto.
2. **Matrícula simultânea em academias diferentes**: um estudante pode, legitimamente, solicitar matrícula em várias academias ao mesmo tempo enquanto decide onde estudar. O problema surge quando uma dessas solicitações é aprovada e o estudante é efetivamente criado (`EstudanteCriadoComVinculo`): as demais solicitações pendentes em outras academias continuam abertas indefinidamente, como se nada tivesse mudado, podendo levar a matrículas duplicadas reais caso outra academia também aprove.

`Lista de tarefas.md` propõe dois mecanismos complementares, e explicitamente reconhece uma limitação: **"Não foi criado um mecanismo para o estudante fazer uma matrícula em mais de uma instituição ao mesmo tempo e escolher em qual ficar porque não existe ainda um mecanismo 100% confiável para garantir que se trata do mesmo estudante"**. Por isso, esta tarefa deve ser tratada como um mecanismo de **melhor esforço** baseado em comparação de dados textuais (nome, data de nascimento, gênero, BI), não como uma resolução de identidade garantida. O primeiro mecanismo (dentro da mesma academia) deve **sinalizar** solicitações semelhantes para revisão humana, sem bloquear automaticamente a criação. O segundo mecanismo (entre academias diferentes) deve agir de forma automática, mas apenas quando houver um identificador mais confiável disponível: o `bilhete_identidade` do próprio estudante.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Duplicidade na mesma academia | Sinalizar, não bloquear | Novo campo na solicitação lista IDs de solicitações semelhantes já pendentes na mesma academia |
| Critério de semelhança | Nome + data de nascimento + gênero, ou BI do estudante, ou BI do responsável | Comparação normalizada (trim, sem diferenciar maiúsculas/minúsculas para BI) |
| Cancelamento entre academias | Automático, apenas com `bilhete_identidade` do estudante preenchido | Ao aprovar uma solicitação com BI do estudante, todas as demais pendentes do mesmo BI em outras academias são canceladas |
| Novo status | `cancelada`, distinto de `reprovada` | Diferencia rejeição por análise documental de cancelamento por matrícula em outra instituição |
| Limitação assumida | Mecanismo de melhor esforço, sem garantia de identidade única | Documentado explicitamente como limitação conhecida do produto |

---

# 1. Sinalizar solicitações semelhantes dentro da mesma academia

## Objetivo

Ao criar uma nova solicitação de matrícula, identificar solicitações pendentes já existentes na mesma academia que pareçam pertencer à mesma pessoa, e registrar essa relação na nova solicitação sem bloquear sua criação.

## Regra de negócio

Ao processar `POST /solicitacao-matricula`, antes de gravar `SolicitacaoMatriculaCriada`, o backend deve:

1. buscar, na mesma `codigo_academia`, solicitações com `status = "pendente"`;
2. comparar cada uma contra os dados da nova solicitação usando os critérios de semelhança definidos em 1.1;
3. para cada solicitação pendente considerada semelhante, adicionar seu `codigo_solicitacao` à lista `solicitacoes_semelhantes` da nova solicitação;
4. prosseguir com a criação normalmente, mesmo que existam solicitações semelhantes — esta etapa **nunca bloqueia** a criação.

## Escopo obrigatório

### 1.1 Critério de semelhança

Duas solicitações pendentes da mesma academia são consideradas semelhantes quando **pelo menos uma** das condições abaixo for verdadeira, após normalização (trim nas extremidades, comparação sem diferenciar maiúsculas/minúsculas para nome e BI):

1. `nome` normalizado igual, `data_nascimento` igual e `genero` igual;
2. `bilhete_identidade` preenchido e igual em ambas;
3. `bilhete_identidade_responsavel` preenchido e igual em ambas.

### 1.2 Novo campo na entidade `SolicitacaoMatricula`

Adicionar o campo público `solicitacoes_semelhantes: string[]` (array de `codigo_solicitacao`), com valor padrão de array vazio quando nenhuma semelhança for encontrada. Este campo deve:

- ser persistido no evento `SolicitacaoMatriculaCriada`;
- ser exposto em `SolicitacaoMatriculaDTO`;
- ser somente-leitura (nunca aceito no payload de criação; calculado exclusivamente pelo backend).

### 1.3 Visibilidade para a academia

`GET /academia/solicitacoes-matricula` e `GET /academia/solicitacao-matricula/:codigo` devem expor `solicitacoes_semelhantes`, permitindo que a academia identifique rapidamente, ao revisar uma solicitação, se já existem outras pendentes possivelmente da mesma pessoa antes de decidir aprovar ou reprovar.

### 1.4 Testes obrigatórios

1. criação de solicitação sem nenhuma semelhante pendente: `solicitacoes_semelhantes = []`;
2. criação de solicitação com nome + data de nascimento + gênero iguais a uma pendente existente: `codigo_solicitacao` da existente aparece na lista;
3. criação de solicitação com `bilhete_identidade` igual a uma pendente existente (mesmo com nome diferente, ex.: erro de digitação): `codigo_solicitacao` aparece na lista;
4. criação de solicitação com `bilhete_identidade_responsavel` igual a uma pendente existente: `codigo_solicitacao` aparece na lista;
5. solicitações semelhantes em **outra** academia não entram na lista (o critério é restrito à mesma `codigo_academia`);
6. solicitações já `aprovada`/`reprovada`/`cancelada` não entram na comparação, apenas `pendente`;
7. a criação da solicitação não é bloqueada em nenhum dos cenários acima.

---

# 2. Cancelar automaticamente solicitações concorrentes entre academias diferentes

## Objetivo

Quando uma solicitação de matrícula com `bilhete_identidade` do próprio estudante for aprovada, cancelar automaticamente todas as demais solicitações pendentes do mesmo estudante em outras academias, evitando matrícula duplicada real.

## Regra de negócio

Ao processar `PUT /academia/solicitacao-matricula/:codigo/aprovar`, depois de gravar `SolicitacaoMatriculaAprovada` e criar o estudante (`EstudanteCriadoComVinculo`), o backend deve:

1. verificar se a solicitação aprovada possui `bilhete_identidade` do estudante preenchido; se **não** possuir, esta etapa é ignorada por completo — o mecanismo só age quando há BI do próprio estudante, por ser um identificador mais confiável do que nome/data de nascimento isolados;
2. se possuir, buscar em **todas as academias** solicitações com `status = "pendente"` cujo `bilhete_identidade` (normalizado) seja igual ao da solicitação recém-aprovada, excluindo a própria;
3. para cada uma encontrada, emitir um evento de cancelamento e atualizar seu `status` para `cancelada`, preenchendo um motivo padronizado indicando que o cancelamento decorre de matrícula aprovada em outra instituição;
4. este cancelamento em cascata não deve remover nem excluir os documentos já enviados dessas solicitações; a política de documentos ao cancelar deve seguir a mesma política já usada para reprovação (remoção do diretório de documentos), salvo decisão explícita em contrário registrada no PR.

## Escopo obrigatório

### 2.1 Novo status `cancelada`

Adicionar `cancelada` ao tipo `SolicitacaoMatriculaStatus` (hoje `'pendente' | 'aprovada' | 'reprovada'`), tornando-o `'pendente' | 'aprovada' | 'reprovada' | 'cancelada'`. Uma solicitação `cancelada` é terminal, assim como `aprovada` e `reprovada`: não pode voltar para `pendente` nem ser aprovada/reprovada posteriormente.

### 2.2 Novo evento `SolicitacaoMatriculaCancelada`

Criar e adicionar à whitelist de eventos autorizados (`safe_queries.go`) o evento `SolicitacaoMatriculaCancelada`, com payload contendo, no mínimo:

```json
{
  "codigo_solicitacao": "string",
  "codigo_academia": "string",
  "motivo": "matricula aprovada em outra instituicao",
  "solicitacao_aprovada_relacionada": "codigo_solicitacao_da_solicitacao_aprovada",
  "codigo_estudante_gerado": "codigo do estudante criado pela solicitacao aprovada"
}
```

### 2.3 Escopo de busca entre academias

A busca por solicitações concorrentes deve varrer `projection_solicitacoes_matricula` **sem filtrar por `codigo_academia`**, já que o objetivo explícito é encontrar solicitações em **outras** instituições. A comparação de `bilhete_identidade` deve usar a mesma normalização já estabelecida no sistema (trim, sem diferenciar maiúsculas/minúsculas).

### 2.4 Atomicidade e resiliência

Se o cancelamento em cascata falhar parcialmente (ex.: erro ao processar uma das solicitações concorrentes), a aprovação da solicitação original e a criação do estudante **não devem ser revertidas** — a matrícula aprovada já é um fato consumado. A falha no cancelamento em cascata deve ser registrada em log/auditoria para correção manual, sem bloquear nem reverter a operação principal.

### 2.5 Consultas e documentação

Atualizar `GET /academia/solicitacoes-matricula`, `GET /academia/solicitacao-matricula/:codigo` e `GET /solicitacoes-matricula` (admin) para aceitar `cancelada` como valor válido do filtro `status`, e atualizar exemplos de resposta.

### 2.6 Testes obrigatórios

1. aprovação de solicitação **sem** `bilhete_identidade` do estudante: nenhuma outra solicitação é cancelada;
2. aprovação de solicitação **com** `bilhete_identidade` do estudante e uma solicitação pendente com o mesmo BI em outra academia: a outra é cancelada com `status = cancelada` e motivo preenchido;
3. aprovação de solicitação com múltiplas solicitações concorrentes pendentes em academias diferentes: todas são canceladas na mesma operação;
4. solicitações já `reprovada` ou já `aprovada` em outras academias não são afetadas pelo cancelamento em cascata;
5. solicitações pendentes com BI **diferente** não são afetadas;
6. uma solicitação `cancelada` não pode ser aprovada nem reprovada posteriormente (`409 Conflict`);
7. falha simulada ao cancelar uma das concorrentes não reverte a aprovação nem a criação do estudante da solicitação original.

---

# 3. Atualização obrigatória da documentação

## Objetivo

Atualizar toda documentação afetada para refletir o novo campo, o novo status e a limitação assumida de identidade.

## Escopo de documentação

Atualizar, quando existirem:

- `Documentação.md`, seção de `SolicitacaoMatricula` (entidade, eventos, processo de negócio, regras de negócio, permissões, endpoints);
- OpenAPI/Swagger, se existir;
- exemplos de payload e resposta de `POST /solicitacao-matricula`, `GET /academia/solicitacoes-matricula`, `GET /academia/solicitacao-matricula/:codigo` e `GET /solicitacoes-matricula`.

## Regras de documentação

A documentação deve declarar explicitamente que:

- `solicitacoes_semelhantes` é calculado automaticamente pelo backend e nunca aceito no payload de criação;
- o critério de semelhança compara nome, data de nascimento, gênero e BI do estudante ou do responsável, dentro da mesma academia;
- o cancelamento automático entre academias diferentes só ocorre quando a solicitação aprovada tiver `bilhete_identidade` do próprio estudante preenchido;
- `cancelada` é um status terminal, distinto de `reprovada`;
- este mecanismo é de melhor esforço e não substitui um sistema de identidade única de estudantes, que ainda não existe na plataforma.

---

# Fora de escopo

- Criar um sistema de identidade única/nacional de estudantes.
- Bloquear automaticamente a criação de uma solicitação por haver semelhantes pendentes na mesma academia (a ação é apenas sinalizar).
- Cancelar solicitações concorrentes quando a solicitação aprovada não tiver `bilhete_identidade` do estudante.
- Permitir que uma solicitação `cancelada` retorne a `pendente`.
- Alterar regras de aprovação/reprovação já existentes além do necessário para o cancelamento em cascata.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `solicitacoes_semelhantes` for calculado corretamente na criação, usando os critérios da seção 1.1, e nunca bloquear a criação;
2. `GET /academia/solicitacoes-matricula` e `GET /academia/solicitacao-matricula/:codigo` exibirem `solicitacoes_semelhantes`;
3. a aprovação de uma solicitação com `bilhete_identidade` do estudante cancelar automaticamente todas as demais pendentes do mesmo BI em outras academias;
4. a aprovação de uma solicitação sem `bilhete_identidade` do estudante não afetar nenhuma outra solicitação;
5. `cancelada` existir como status terminal, com evento `SolicitacaoMatriculaCancelada` auditável e presente na whitelist;
6. falha parcial no cancelamento em cascata não reverter a aprovação nem a criação do estudante;
7. `Documentação.md` e demais materiais afetados estarem atualizados, incluindo a limitação assumida de identidade;
8. testes automatizados cobrirem os cenários das seções 1.4 e 2.6;
9. o PR explicar claramente as mudanças de contrato e a limitação assumida do mecanismo.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Prevenir e sinalizar matrícula duplicada em múltiplas instituições (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
