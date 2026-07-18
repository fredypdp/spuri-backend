---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Reforçar validações na edição de dados cadastrais dos usuários (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento auditando `PUT /academia/dados`, `PUT /estudante/dados-pessoais` e `PUT /dominis/admin/:id/dados`, corrigindo a inconsistência de reset de `email_verificado`/`telefone_verificado` entre elas e, principalmente, removendo de `PUT /academia/dados` a possibilidade de alterar `anos_academicos`, `cursos`, `type` e `nivel_escolar` sem passar pelas validações já existentes em rotas dedicadas. Ao final, atualize testes, documentação técnica e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou caminhos alternativos que permitam contornar as validações das rotas dedicadas.

## Contexto

O sistema já possui três rotas de edição de dados cadastrais próprios: `PUT /academia/dados` (academia), `PUT /estudante/dados-pessoais` (estudante) e `PUT /dominis/admin/:id/dados` (admin). Duas lacunas concretas foram identificadas na comparação entre elas e o restante do sistema já documentado:

**Primeira lacuna — reconstrução de campos que já têm rota dedicada.** `Documentação.md` mostra `PUT /academia/dados` aceitando, no mesmo payload de texto livre, os campos `anos_academicos` e `cursos`. Isso é inconsistente com o resto do sistema:

- `anos_academicos` da academia já possui rotas dedicadas e cuidadosamente validadas — `POST /academia/anos-academicos` e `DELETE /academia/anos-academicos` — implementadas justamente para impedir "operações 'replace all' que removam anos silenciosamente" e para bloquear remoção com `409 Conflict` quando há estudantes ativos vinculados ao ano (ver `docs/Tarefas feitas/Permitir às academias adicionar ou remover anos acadêmicos com validações avançadas.md`). Se `PUT /academia/dados` ainda aceitar `anos_academicos` como substituição direta de lista, ele reabre exatamente a brecha que aquela tarefa fechou deliberadamente.
- Cursos já são geridos por `POST /academia/curso`, `PUT /academia/curso/:id/dados`, `PUT /academia/curso/:id/ativar`/`desativar` e `DELETE /academia/curso/:id`, com todas as regras de ciclo de vida documentadas na seção 10. O campo `cursos: string[]` em `PUT /academia/dados` é apenas uma lista de nomes e não deve ser tratado como fonte de verdade paralela ao cadastro real de cursos.
- `type` (`AcademiaType`: `'public'|'private'`) e `nivel_escolar` (`NivelEscolar`: `'fundamental'|'medio'|'misto'`) também aparecem no payload de `PUT /academia/dados` sem exigência de documento comprobativo nem validação de impacto em dados dependentes (estudantes, cursos e `anos_academicos` já vinculados ao `nivel_escolar` atual). A tarefa "Permitir academia alterar type e nível escolar mediante documento" (arquivo `07`) cria o fluxo correto e seguro para essas duas mudanças; esta tarefa (`06`) é responsável por **remover** a possibilidade de alterá-las pelo caminho genérico e inseguro enquanto o fluxo dedicado não existir.

**Segunda lacuna — inconsistência de reset de verificação.** `Documentação.md` afirma, especificamente na seção de `PUT /academia/dados`: "se o email for alterado, `email_verificado` volta para `false`; se o telefone for alterado, `telefone_verificado` volta para `false`." A mesma garantia não está documentada para `PUT /estudante/dados-pessoais`, que também permite alterar `email` e `telefone`. Sem essa auditoria, um estudante poderia alterar seu email/telefone sem que o sistema volte a exigir nova verificação, criando uma inconsistência de segurança entre os dois fluxos.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| `anos_academicos` em `PUT /academia/dados` | Remover; direcionar para `/academia/anos-academicos` | Nenhuma alteração de anos acadêmicos passa fora das validações dedicadas |
| `cursos` em `PUT /academia/dados` | Remover; direcionar para `/academia/curso` | Nenhuma alteração de curso passa fora do ciclo de vida já documentado |
| `type` e `nivel_escolar` em `PUT /academia/dados` | Remover temporariamente até a tarefa 07 existir | Fecha a brecha de mudança sem documento comprobativo nem validação de impacto |
| Reset de verificação | Padronizar entre `PUT /academia/dados` e `PUT /estudante/dados-pessoais` | Alterar email/telefone sempre reseta o respectivo `*_verificado` para `false`, nos dois fluxos |
| Hierarquia de edição de admin | Auditar `PUT /dominis/admin/:id/dados` contra a regra de hierarquia estrita já documentada | Confirmar (ou corrigir) que só é possível editar admin de role estritamente inferior |

---

# 1. Remover de `PUT /academia/dados` os campos com rota dedicada

## Objetivo

Impedir que `PUT /academia/dados` altere `anos_academicos`, `cursos`, `type` e `nivel_escolar`, reservando essas mudanças para as rotas já validadas para esse fim (`/academia/anos-academicos`, `/academia/curso`) ou para o fluxo dedicado ainda a ser criado (tarefa 07, para `type`/`nivel_escolar`).

## Regra de negócio

`PUT /academia/dados` deve continuar aceitando apenas: `nome`, `provincia`, `endereco`, `telefone`, `email`, `website`. Qualquer payload contendo `anos_academicos`, `cursos`, `type` ou `nivel_escolar` deve ser rejeitado com erro de validação claro, orientando a rota correta para cada campo, sem mutação parcial.

## Escopo obrigatório

### 1.1 Rejeitar explicitamente, não apenas ignorar

Seguindo o padrão já adotado em outras rotas do sistema (ex.: `PUT /academia/curso/:id/dados` já rejeita `anos_academicos`, `periodos`, `semestres` com erro de validação, sem mutação parcial), `PUT /academia/dados` deve **rejeitar** esses campos com `400` quando presentes no payload, e não apenas ignorá-los silenciosamente. Ignorar silenciosamente esconderia do cliente que a mudança não teve efeito.

### 1.2 Mensagens de erro orientando a rota correta

```json
{
  "error": "VALIDATION_ERROR",
  "message": "O campo 'anos_academicos' não é aceito em PUT /academia/dados. Use POST/DELETE /academia/anos-academicos para adicionar ou remover anos acadêmicos.",
  "details": [{"field": "anos_academicos", "code": "campo_nao_permitido", "message": "..."}]
}
```

O mesmo padrão deve ser aplicado para `cursos`, `type` e `nivel_escolar`, cada um apontando para a rota/tarefa correta (`/academia/curso` para `cursos`; para `type`/`nivel_escolar`, a mensagem deve indicar que a alteração exige documento comprobativo pelo fluxo dedicado — ver tarefa 07 — enquanto esse fluxo não existir, a mensagem deve deixar claro que a alteração está temporariamente indisponível por este caminho).

### 1.3 Testes obrigatórios

1. `PUT /academia/dados` com `nome`/`provincia`/`endereco`/`telefone`/`email`/`website`: sucesso, sem regressão;
2. `PUT /academia/dados` com `anos_academicos`: rejeitado com `400`, sem alterar nenhum outro campo do payload;
3. `PUT /academia/dados` com `cursos`: rejeitado com `400`;
4. `PUT /academia/dados` com `type`: rejeitado com `400`;
5. `PUT /academia/dados` com `nivel_escolar`: rejeitado com `400`;
6. `PUT /academia/dados` com combinação de campo permitido e campo rejeitado no mesmo payload: nenhum campo é alterado (sem mutação parcial).

---

# 2. Padronizar reset de verificação de email/telefone

## Objetivo

Garantir que alterar `email` ou `telefone` reseta o respectivo `email_verificado`/`telefone_verificado` para `false` de forma idêntica em `PUT /academia/dados` e `PUT /estudante/dados-pessoais`.

## Regra de negócio

Em qualquer rota de edição de dados cadastrais que permita alterar `email` ou `telefone`/`telefone_responsavel`, uma mudança efetiva de valor (o novo valor é diferente do anterior, após normalização) deve resetar automaticamente a respectiva flag de verificação para `false`, exigindo nova verificação.

## Escopo obrigatório

### 2.1 Auditar `PUT /estudante/dados-pessoais`

Confirmar, com teste, se hoje alterar `email` reseta `email_verificado` e se alterar `telefone` reseta `telefone_verificado`. Se a auditoria confirmar que isso **não** acontece, corrigir para reproduzir exatamente o comportamento já documentado para `PUT /academia/dados`.

### 2.2 Considerar `telefone_responsavel`

Avaliar se alterar `telefone_responsavel` do estudante deve resetar `telefone_responsavel_verificado`, mantendo a mesma lógica, mesmo que a verificação de telefone ainda não esteja ativa como fluxo funcional no sistema (`Documentação.md` já registra que os campos `*_verificado` existem na estrutura, mas nenhum fluxo de verificação está ativo). A consistência do reset deve ser mantida independentemente de o fluxo de verificação estar ativo, para que o campo já esteja correto quando a verificação for implementada.

### 2.3 Testes obrigatórios

1. estudante altera `email` para um valor diferente: `email_verificado` volta para `false`;
2. estudante reenvia o mesmo `email` já cadastrado (sem mudança real): `email_verificado` não é alterado;
3. estudante altera `telefone`: `telefone_verificado` volta para `false`;
4. estudante altera `telefone_responsavel`: `telefone_responsavel_verificado` volta para `false`;
5. academia altera `email`/`telefone`: comportamento já documentado continua funcionando sem regressão.

---

# 3. Auditar hierarquia de edição de dados de admin

## Objetivo

Confirmar que `PUT /dominis/admin/:id/dados` respeita a regra de hierarquia estrita já documentada ("Só pode gerenciar roles estritamente inferiores"), e corrigir caso não respeite.

## Escopo obrigatório

### 3.1 Auditoria

Confirmar, com teste, se um admin `gerente` consegue editar nome/email de outro admin `gerente` ou de um admin `adm`/`fpp` via `PUT /dominis/admin/:id/dados`. Comparar o comportamento contra a mesma regra de hierarquia já aplicada em `PUT /dominis/admin/:id/role`, `PUT /dominis/admin/:id/ativar` e `PUT /dominis/admin/:id/desativar`.

### 3.2 Corrigir se necessário

Se a auditoria confirmar que `PUT /dominis/admin/:id/dados` não aplica a mesma hierarquia, corrigir para exigir que o admin executante tenha role estritamente superior ao admin alvo, com a mesma exceção documentada de auto-edição (um admin pode editar os próprios `nome`/`email`, se essa exceção já existir para outras ações administrativas equivalentes; documentar explicitamente a decisão adotada).

### 3.3 Testes obrigatórios

1. admin `fpp` edita dados de admin `gerente`: sucesso;
2. admin `gerente` tenta editar dados de admin `fpp` ou `adm`: rejeitado com `403`;
3. admin `gerente` tenta editar dados de outro admin `gerente`: comportamento decidido e testado conforme 3.2;
4. admin edita os próprios dados: comportamento decidido e testado conforme 3.2.

---

# Fora de escopo

- Implementar o fluxo dedicado de alteração de `type`/`nivel_escolar` com documento comprobativo (tarefa 07).
- Implementar verificação funcional de telefone (permanece fora de escopo do produto, conforme já documentado).
- Alterar a lógica de `/academia/anos-academicos` ou `/academia/curso` além do necessário para direcionar os erros de `PUT /academia/dados`.
- Criar aliases, wrappers de compatibilidade ou parâmetro opcional que reative os campos removidos de `PUT /academia/dados`.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. `PUT /academia/dados` rejeitar `anos_academicos`, `cursos`, `type` e `nivel_escolar` com erro claro, sem mutação parcial;
2. `PUT /academia/dados` continuar aceitando normalmente `nome`, `provincia`, `endereco`, `telefone`, `email`, `website`;
3. `PUT /estudante/dados-pessoais` resetar `email_verificado`/`telefone_verificado`/`telefone_responsavel_verificado` de forma consistente com `PUT /academia/dados`;
4. a hierarquia de `PUT /dominis/admin/:id/dados` estar auditada e, se necessário, corrigida para respeitar a regra já documentada;
5. `Documentação.md` refletir o novo contrato de `PUT /academia/dados` e o comportamento auditado das demais rotas;
6. testes automatizados cobrirem os cenários das seções 1.3, 2.3 e 3.3;
7. o PR explicar claramente a brecha fechada e sua relação com a tarefa 07.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Reforçar validações na edição de dados cadastrais dos usuários (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
