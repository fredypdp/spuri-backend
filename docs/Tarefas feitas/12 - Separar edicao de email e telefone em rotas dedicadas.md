---
criado: 2026-07-24 00:00
origem: solicitação direta
status: feito
---

# Separar edição de email e telefone em rotas dedicadas (feito)

## Prompt recomendado para executar a atualização

Implemente rotas dedicadas para alteração de email e telefone de todos os tipos de usuário autenticados, usando o token da requisição para identificar se o solicitante é estudante, academia ou admin. Remova a edição de `email` e `telefone` dos endpoints genéricos de dados cadastrais, preserve o reset das flags de verificação quando houver mudança real de valor e valide telefone aceitando somente dígitos nacionais, sem DDI, sem `+`, sem espaços e sem caracteres de formatação. Ao final, atualize testes, documentação técnica e qualquer documentação afetada. Não criar aliases, wrappers de compatibilidade ou caminhos alternativos que permitam alterar email/telefone por rotas genéricas.

## Contexto

O sistema já possui endpoints genéricos de edição de dados cadastrais, como `PUT /academia/dados`, `PUT /estudante/dados-pessoais` e `PUT /dominis/admin/:id/dados`. A tarefa `06 - Reforçar validações na edição de dados cadastrais dos usuários.md` já tratou a necessidade de reforçar campos sensíveis e padronizar o reset de `email_verificado`/`telefone_verificado` quando email ou telefone mudarem.

Esta tarefa aprofunda essa separação: email e telefone deixam de ser campos editáveis dentro de rotas genéricas e passam a ter rotas próprias. O objetivo é reduzir ambiguidade, centralizar validações de contato e facilitar a evolução futura de fluxos de confirmação/verificação sem replicar lógica em múltiplos handlers.

A identificação do tipo de usuário **não** deve depender de parâmetro enviado pelo cliente no corpo da requisição. A rota deve usar o token/autenticação já existente para descobrir se quem chamou é estudante, academia ou admin e aplicar a atualização no agregado/projeção correspondente.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Rotas dedicadas | Criar uma rota para editar email e outra para editar telefone | Email e telefone deixam de ser alterados por endpoints genéricos |
| Escopo de usuário | Atende estudantes, academias e admins autenticados | O token define qual entidade será atualizada |
| Telefone | Aceitar somente dígitos/números nacionais, sem DDI | Payloads com `+244`, espaços, hífens, parênteses ou letras são rejeitados |
| Verificação | Mudança real reseta a flag correspondente | `email_verificado`/`telefone_verificado` voltam para `false` quando o contato muda |
| Compatibilidade | Não manter caminhos antigos para contato | Nenhuma rota genérica continua alterando email/telefone |

---

# 1. Criar rotas dedicadas para edição de email e telefone

## Objetivo

Disponibilizar dois endpoints autenticados e centralizados para alteração de contatos próprios:

- uma rota dedicada para editar email;
- uma rota dedicada para editar telefone.

A nomenclatura final deve seguir o padrão do roteador atual do projeto. Como referência de contrato, recomenda-se um agrupamento neutro por usuário autenticado, por exemplo:

- `PUT /me/email`;
- `PUT /me/telefone`.

Se o padrão existente do backend indicar outro prefixo mais consistente, usar esse padrão, desde que as duas rotas continuem sendo únicas, dedicadas e válidas para todos os tipos de usuário autenticados.

## Regra de negócio

A rota deve usar exclusivamente o token da requisição para identificar o usuário autenticado e o tipo de entidade que será atualizada. O cliente não deve enviar `tipo_usuario`, `role`, `academia_id`, `estudante_id` ou `admin_id` no corpo para escolher o alvo da alteração.

## Escopo obrigatório

### 1.1 Contrato de `PUT /me/email`

Payload recomendado:

```json
{
  "email": "novo.email@exemplo.com"
}
```

Comportamento esperado:

1. exigir autenticação;
2. identificar pelo token se o usuário é estudante, academia ou admin;
3. validar formato de email com o validador já utilizado no projeto;
4. verificar unicidade conforme as regras atuais de cadastro/edição de email;
5. se o email for igual ao atual após normalização, não resetar indevidamente a flag de verificação;
6. se o email for diferente do atual, persistir o novo email e resetar `email_verificado` para `false` quando a entidade possuir esse campo;
7. retornar a entidade/DTO atualizado ou resposta padronizada já usada em rotas de edição equivalentes.

### 1.2 Contrato de `PUT /me/telefone`

Payload recomendado:

```json
{
  "telefone": "923456789"
}
```

Comportamento esperado:

1. exigir autenticação;
2. identificar pelo token se o usuário é estudante, academia ou admin;
3. aceitar somente string composta por dígitos (`0-9`);
4. rejeitar qualquer DDI ou formatação, incluindo `+244923456789`, `244923456789`, `923 456 789`, `923-456-789`, `(923)456789` e valores com letras;
5. aplicar validação de tamanho/formato nacional já adotada pelo domínio, se existir; se não existir, definir e documentar explicitamente a regra nacional mínima;
6. verificar unicidade conforme as regras atuais de cadastro/edição de telefone, se aplicável;
7. se o telefone for igual ao atual após normalização, não resetar indevidamente a flag de verificação;
8. se o telefone for diferente do atual, persistir o novo telefone e resetar `telefone_verificado` para `false` quando a entidade possuir esse campo;
9. retornar a entidade/DTO atualizado ou resposta padronizada já usada em rotas de edição equivalentes.

### 1.3 Tipos de usuário cobertos

As duas rotas devem cobrir, no mínimo:

1. estudante autenticado;
2. academia autenticada;
3. admin autenticado.

Se o token atual distinguir subtipos de admin (`adm`, `fpp`, `gerente` etc.), essa distinção deve continuar existindo para autorização administrativa, mas não deve alterar o fato de que o admin autenticado edita o próprio email/telefone por estas rotas.

---

# 2. Remover email e telefone das rotas genéricas

## Objetivo

Impedir que rotas genéricas de edição cadastral continuem alterando email ou telefone, forçando o uso dos endpoints dedicados.

## Regra de negócio

Qualquer payload enviado a uma rota genérica de dados cadastrais contendo `email` ou `telefone` deve ser rejeitado com erro de validação claro, sem mutação parcial. Não basta ignorar silenciosamente os campos.

## Escopo obrigatório

### 2.1 Rotas a auditar

Auditar e ajustar, no mínimo:

1. `PUT /academia/dados`;
2. `PUT /estudante/dados-pessoais`;
3. `PUT /dominis/admin/:id/dados`.

Se houver outras rotas genéricas que atualmente editem `email`, `telefone`, `telefone_encarregado` ou campos equivalentes de contato, elas também devem ser ajustadas ou documentadas explicitamente como exceção justificada.

### 2.2 Erro esperado para campos não permitidos

O erro deve orientar a rota correta, por exemplo:

```json
{
  "error": "VALIDATION_ERROR",
  "message": "O campo 'email' não é aceito nesta rota. Use PUT /me/email para alterar o email do usuário autenticado.",
  "details": [
    {
      "field": "email",
      "code": "campo_nao_permitido",
      "message": "Use PUT /me/email para alterar o email."
    }
  ]
}
```

Para telefone, a mensagem deve apontar para `PUT /me/telefone`.

### 2.3 Sem mutação parcial

Se o payload misturar campos permitidos e campos de contato proibidos, a requisição inteira deve falhar e nenhum campo deve ser atualizado.

---

# 3. Validar telefone sem DDI e somente com dígitos

## Objetivo

Padronizar a regra de telefone para edição: o backend deve receber apenas o número nacional, composto exclusivamente por dígitos, sem código do país e sem caracteres de apresentação.

## Regra de negócio

A rota dedicada de telefone deve rejeitar qualquer valor que não case com a regra `^[0-9]+$` e deve rejeitar DDI. A regra deve ser aplicada antes de qualquer normalização que pudesse transformar silenciosamente um valor inválido em válido.

## Escopo obrigatório

### 3.1 Casos válidos

Exemplos de entradas válidas, desde que também passem no tamanho/formato nacional definido pelo domínio:

```json
{ "telefone": "923456789" }
```

### 3.2 Casos inválidos obrigatórios

Devem ser rejeitados com `400` e erro de validação:

1. `+244923456789`;
2. `244923456789`, quando representar DDI + número nacional;
3. `923 456 789`;
4. `923-456-789`;
5. `(923)456789`;
6. `923abc789`;
7. valor numérico JSON sem aspas, se o contrato definir telefone como string;
8. string vazia ou composta apenas por espaços.

### 3.3 Mensagem de erro

A mensagem deve explicar que o telefone deve conter apenas dígitos do número nacional e não deve incluir DDI.

---

# 4. Event sourcing, projeções e auditoria

## Objetivo

Manter o padrão arquitetural do projeto para mudanças de estado sensíveis.

## Regra de negócio

Alterações de email e telefone devem seguir o padrão de persistência/event sourcing já usado para atualizações cadastrais. Se hoje as alterações equivalentes geram eventos, as novas rotas devem gerar eventos específicos ou equivalentes. Se a atualização atual ainda não registra evento para algum tipo de usuário, a decisão deve ser auditada e documentada durante a implementação.

## Escopo obrigatório

1. criar ou reutilizar comandos/agregados adequados para alteração de email;
2. criar ou reutilizar comandos/agregados adequados para alteração de telefone;
3. garantir rebuild correto das projeções afetadas;
4. preservar `email_verificado`/`telefone_verificado` em alteração sem mudança real;
5. resetar `email_verificado`/`telefone_verificado` em alteração com mudança real;
6. documentar qualquer diferença entre entidades que possuam ou não possuam flags de verificação.

---

# 5. Testes obrigatórios

## Objetivo

Cobrir o novo contrato, a remoção dos caminhos antigos e a validação estrita de telefone.

## Cenários mínimos

### 5.1 Email

1. estudante autenticado altera o próprio email por rota dedicada com sucesso;
2. academia autenticada altera o próprio email por rota dedicada com sucesso;
3. admin autenticado altera o próprio email por rota dedicada com sucesso;
4. email inválido é rejeitado;
5. email duplicado é rejeitado conforme regra de unicidade existente;
6. mudança real de email reseta `email_verificado` quando aplicável;
7. reenviar o mesmo email não reseta indevidamente `email_verificado`.

### 5.2 Telefone

1. estudante autenticado altera o próprio telefone por rota dedicada com sucesso;
2. academia autenticada altera o próprio telefone por rota dedicada com sucesso;
3. admin autenticado altera o próprio telefone por rota dedicada com sucesso, se admin possuir telefone no modelo atual;
4. telefone com DDI `+244` é rejeitado;
5. telefone iniciando por `244` como DDI é rejeitado quando exceder o formato nacional esperado;
6. telefone com espaços, hífens, parênteses ou letras é rejeitado;
7. telefone enviado como número JSON, e não string, é rejeitado se o contrato for string;
8. mudança real de telefone reseta `telefone_verificado` quando aplicável;
9. reenviar o mesmo telefone não reseta indevidamente `telefone_verificado`.

### 5.3 Rotas genéricas

1. `PUT /academia/dados` com `email` é rejeitado sem mutação parcial;
2. `PUT /academia/dados` com `telefone` é rejeitado sem mutação parcial;
3. `PUT /estudante/dados-pessoais` com `email` é rejeitado sem mutação parcial;
4. `PUT /estudante/dados-pessoais` com `telefone` é rejeitado sem mutação parcial;
5. `PUT /dominis/admin/:id/dados` com `email` é rejeitado sem mutação parcial;
6. `PUT /dominis/admin/:id/dados` com `telefone` é rejeitado sem mutação parcial, se o campo existir para admin;
7. payload misto com campo permitido e `email`/`telefone` proibido não altera nenhum dado.

---

# Fora de escopo

- Implementar envio real de email, SMS ou OTP de verificação.
- Alterar a política de autenticação, emissão de token ou refresh token além do necessário para ler o tipo de usuário autenticado.
- Permitir que um admin altere email/telefone de outro usuário por estas rotas próprias; estas rotas são para o usuário autenticado alterar o próprio contato.
- Criar compatibilidade com os campos antigos nas rotas genéricas.
- Aceitar DDI, normalizar `+244` automaticamente ou remover caracteres de formatação silenciosamente.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. existir uma rota dedicada para editar email do usuário autenticado;
2. existir uma rota dedicada para editar telefone do usuário autenticado;
3. as duas rotas identificarem o tipo de usuário pelo token, sem depender de campo enviado pelo cliente para escolher o alvo;
4. estudantes, academias e admins estiverem cobertos conforme o modelo atual permitir;
5. telefone aceitar somente dígitos do número nacional, sem DDI e sem caracteres de formatação;
6. rotas genéricas não aceitarem mais `email`/`telefone`, rejeitando com erro claro e sem mutação parcial;
7. mudanças reais de email/telefone resetarem as flags de verificação correspondentes quando aplicável;
8. reenvio do mesmo email/telefone não resetar flags indevidamente;
9. `Documentação.md` refletir o novo contrato e remover os exemplos antigos de edição de email/telefone em rotas genéricas;
10. testes automatizados cobrirem os cenários da seção 5.

## Procedimento de conclusão

Ao finalizar a implementação:

1. atualizar o título interno desta tarefa para `# Separar edição de email e telefone em rotas dedicadas (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.
