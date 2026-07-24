---
criado: 2026-07-24 00:00
origem: solicitação direta
status: concluida
---

# Criar solicitações documentadas para edição de dados sensíveis do estudante

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento criando um mecanismo de solicitação de edição, iniciado pelo estudante autenticado e acompanhado de documento comprobativo temporário, para alterar separadamente `nome` do estudante, `bilhete_identidade` do estudante, `bilhete_identidade_encarregado` e `data_nascimento` do estudante. Cada campo deve ter uma rota dedicada para criar a solicitação e uma rota dedicada para executar a edição após aprovação, sem aceitar esses campos em rotas genéricas. O documento enviado deve ficar em diretório temporário e deve ser removido do armazenamento depois que a solicitação for aprovada ou reprovada. Além disso, a edição do telefone do encarregado deve ter rota própria/dedicada. Ao final, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou fallbacks temporários.

## Contexto

Dados de identificação civil do estudante e do encarregado são campos sensíveis: alteração incorreta de nome, bilhete de identidade ou data de nascimento pode comprometer histórico acadêmico, emissão de documentos e rastreabilidade do estudante. Por isso, esses campos não devem ser editados diretamente por uma rota genérica de dados pessoais nem misturados num payload amplo.

A regra desejada é semelhante aos fluxos com aprovação e armazenamento temporário já documentados para documentos formais: o estudante solicita a alteração, envia um PDF que justifica a mudança, e a academia decide. A alteração efetiva só ocorre após aprovação, por uma rota própria para o campo aprovado. Se a solicitação for reprovada, o dado vigente permanece intacto. Em ambos os casos, o arquivo temporário deve ser deletado do storage ao final da decisão.

Também há uma separação adicional necessária para contato do encarregado: `telefone_encarregado` não deve continuar dependente de rota genérica de dados pessoais. Ele deve ter rota dedicada, seguindo a mesma filosofia já adotada para email e telefone do usuário autenticado.

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Solicitante | Estudante autenticado | O próprio estudante inicia pedidos para alterar seus dados sensíveis |
| Campos com solicitação | `nome`, `bilhete_identidade`, `bilhete_identidade_encarregado`, `data_nascimento` | Cada campo possui fluxo isolado, auditável e documentado |
| Rotas de solicitação | Uma rota dedicada por campo | Não há endpoint genérico para criar solicitações sensíveis |
| Rotas de edição | Uma rota dedicada por campo | A alteração aprovada é aplicada por comando específico do campo |
| Documento comprovativo | PDF obrigatório em storage temporário | Evidência fica disponível durante análise e é removida após decisão |
| Aprovação | Academia vinculada ao estudante | Dado vigente é alterado apenas após aprovação explícita |
| Reprovação | Academia vinculada ao estudante | Dado vigente permanece inalterado e documento temporário é deletado |
| Telefone do encarregado | Rota dedicada sem solicitação documental | Edição de contato fica fora da rota genérica de dados pessoais |

---

# 1. Criar entidade de solicitação de edição de dados sensíveis

## Objetivo

Modelar cada pedido como uma entidade auditável, com ciclo de vida `pendente → aprovada`/`reprovada`, seguindo o padrão de Event Sourcing + CQRS usado no repositório.

## Regra de negócio

### 1.1 Campos da entidade

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `id` | UUID | Sim | Identificador interno da solicitação |
| `codigo_solicitacao` | string | Sim | Código curto único para uso em rotas e consultas |
| `codigo_estudante` | string | Sim | Estudante dono do dado a alterar |
| `codigo_academia` | string | Sim | Academia responsável pela aprovação |
| `campo` | enum | Sim | `nome`, `bilhete_identidade`, `bilhete_identidade_encarregado` ou `data_nascimento` |
| `valor_atual` | string | Sim | Valor vigente no momento da solicitação, para auditoria |
| `valor_solicitado` | string | Sim | Novo valor pretendido |
| `documento_temporario_path` | string | Sim | Caminho do PDF enviado para análise |
| `documento_temporario_url` | string | Não | URL pública/assinada quando o provider suportar |
| `status` | enum | Sim | `pendente`, `aprovada` ou `reprovada` |
| `motivo_reprovacao` | string | Não | Obrigatório ao reprovar |
| `solicitado_por` | string | Sim | Código do estudante autenticado |
| `decidido_por` | string/UUID | Não | Identificador da academia que aprovou/reprovou |
| `created_at`/`updated_at` | RFC3339 | Sim | Datas de criação e atualização |
| `version` | int | Sim | Versão do aggregate |

### 1.2 Eventos do ledger

Adicionar eventos explícitos e permitidos na whitelist de eventos seguros:

- `SolicitacaoEdicaoDadoEstudanteCriada`
- `SolicitacaoEdicaoDadoEstudanteAprovada`
- `SolicitacaoEdicaoDadoEstudanteReprovada`
- `NomeEstudanteAlteradoPorSolicitacao`
- `BilheteIdentidadeEstudanteAlteradoPorSolicitacao`
- `BilheteIdentidadeEncarregadoAlteradoPorSolicitacao`
- `DataNascimentoEstudanteAlteradaPorSolicitacao`
- `TelefoneEncarregadoAlterado`

A aprovação deve registrar evento na solicitação e evento correspondente no ledger do estudante, para que o histórico do estudante explique exatamente qual campo mudou, por qual solicitação e por qual documento analisado.

## Escopo obrigatório

### 1.3 Um pedido pendente por campo

Não permitir mais de uma solicitação `pendente` para o mesmo `codigo_estudante` + `campo`. Uma nova tentativa enquanto já houver solicitação pendente para o mesmo campo deve retornar `409 Conflict`.

### 1.4 Validação do arquivo

O documento comprovativo deve seguir a mesma regra de documentos PDF do sistema: `Content-Type: application/pdf`, extensão `.pdf`, assinatura `%PDF` e limite máximo de 10MB.

### 1.5 Storage temporário

Salvar o PDF em diretório temporário isolado, por exemplo:

```text
{codigo_academia}/estudantes/{codigo_estudante}/edicoes_dados_pendentes/{campo}_{codigo_solicitacao}.pdf
```

O arquivo temporário deve ser deletado do armazenamento depois de qualquer decisão terminal (`aprovada` ou `reprovada`).

---

# 2. Criar rotas dedicadas para solicitação pelo estudante

## Objetivo

Garantir que cada campo sensível tenha uma rota própria de solicitação, com payload mínimo, validação específica e documento obrigatório.

## Regra de negócio

Criar as rotas abaixo, protegidas por autenticação de estudante:

- `POST /estudante/solicitacoes-edicao/nome`
- `POST /estudante/solicitacoes-edicao/bilhete-identidade`
- `POST /estudante/solicitacoes-edicao/bilhete-identidade-encarregado`
- `POST /estudante/solicitacoes-edicao/data-nascimento`

Cada rota deve aceitar `multipart/form-data` com:

| Campo | Obrigatório | Descrição |
| --- | --- | --- |
| `novo_valor` | Sim | Valor pretendido para o campo da rota |
| `documento` | Sim | PDF comprovativo |

O backend deve:

1. identificar o estudante exclusivamente pelo token;
2. validar que o estudante está vinculado a uma academia ativa;
3. validar o `novo_valor` conforme o campo da rota;
4. validar e enviar o PDF para o diretório temporário;
5. criar a solicitação apenas após upload bem-sucedido;
6. em falha posterior à criação do arquivo, tentar remover o arquivo temporário para evitar órfãos.

## Escopo obrigatório

### 2.1 Validações por campo

- `nome`: obrigatório, trim, tamanho mínimo/máximo compatível com o padrão atual de nomes do sistema e rejeição de valor igual ao vigente.
- `bilhete_identidade`: obrigatório, validado pelo mesmo validador atual de BI/NIF/identificadores aplicável ao estudante, com rejeição de duplicidade quando já existir outro estudante com o mesmo BI.
- `bilhete_identidade_encarregado`: obrigatório, validado pelo mesmo padrão atual de BI do encarregado, com rejeição de valor igual ao vigente.
- `data_nascimento`: obrigatória, formato de data aceito pelo sistema, idade e coerência temporal validadas pelo mesmo padrão do cadastro de estudante, com rejeição de valor igual ao vigente.

### 2.2 Testes obrigatórios

1. estudante cria solicitação válida para cada um dos quatro campos;
2. estudante sem academia vinculada tenta solicitar alteração e recebe erro;
3. arquivo inválido ou acima do limite é rejeitado sem criar solicitação;
4. valor igual ao vigente é rejeitado;
5. segunda solicitação pendente para o mesmo campo retorna `409 Conflict`;
6. falha simulada de upload não cria solicitação;
7. payload não consegue escolher outro estudante, academia ou campo diferente da rota.

---

# 3. Criar rotas dedicadas de decisão pela academia

## Objetivo

Permitir que a academia vinculada decida as solicitações pendentes de seus estudantes, sem permitir decisão por academia alheia.

## Regra de negócio

Criar rotas protegidas por autenticação de academia ativa:

- `PUT /academia/solicitacoes-edicao-estudante/nome/:codigo/aprovar`
- `PUT /academia/solicitacoes-edicao-estudante/nome/:codigo/reprovar`
- `PUT /academia/solicitacoes-edicao-estudante/bilhete-identidade/:codigo/aprovar`
- `PUT /academia/solicitacoes-edicao-estudante/bilhete-identidade/:codigo/reprovar`
- `PUT /academia/solicitacoes-edicao-estudante/bilhete-identidade-encarregado/:codigo/aprovar`
- `PUT /academia/solicitacoes-edicao-estudante/bilhete-identidade-encarregado/:codigo/reprovar`
- `PUT /academia/solicitacoes-edicao-estudante/data-nascimento/:codigo/aprovar`
- `PUT /academia/solicitacoes-edicao-estudante/data-nascimento/:codigo/reprovar`

A rota deve validar que o `:codigo` pertence a uma solicitação do campo correspondente à rota e que o estudante está vinculado à academia autenticada.

### 3.1 Aprovação

Ao aprovar:

1. validar que a solicitação ainda está `pendente`;
2. revalidar o `valor_solicitado` contra o estado atual para evitar aprovar dado que se tornou inválido;
3. aplicar a alteração do campo por comando/evento dedicado no aggregate do estudante;
4. gravar `SolicitacaoEdicaoDadoEstudanteAprovada`;
5. deletar o PDF temporário do storage após a decisão terminal;
6. retornar os dados atualizados do estudante ou resumo da solicitação aprovada.

### 3.2 Reprovação

Ao reprovar:

1. exigir `motivo_reprovacao` não vazio;
2. validar que a solicitação ainda está `pendente`;
3. gravar `SolicitacaoEdicaoDadoEstudanteReprovada`;
4. deletar o PDF temporário do storage;
5. manter o dado vigente do estudante inalterado.

## Escopo obrigatório

### 3.3 Solicitação decidida é terminal

Solicitação `aprovada` ou `reprovada` não pode ser aprovada/reprovada novamente. Tentativas subsequentes devem retornar `409 Conflict`.

### 3.4 Falha ao deletar documento temporário

A decisão de negócio não deve ficar parcialmente invisível. Se a alteração/aprovação ou reprovação for gravada com sucesso, mas a deleção do arquivo temporário falhar por limitação do storage, o backend deve registrar erro operacional observável e expor estado suficiente para reprocessamento seguro, sem restaurar o dado já aprovado. Preferir operação transacional/compensatória quando o provider permitir.

### 3.5 Testes obrigatórios

1. academia aprova cada um dos quatro campos e o estudante recebe o novo valor;
2. academia reprova cada um dos quatro campos e o estudante mantém o valor anterior;
3. academia de outra instituição tenta decidir solicitação e recebe `403`;
4. reprovação sem `motivo_reprovacao` é rejeitada;
5. tentativa de decidir solicitação já decidida retorna `409 Conflict`;
6. rota de `nome` não aprova solicitação de `data_nascimento` e vice-versa;
7. arquivo temporário é deletado depois de aprovação;
8. arquivo temporário é deletado depois de reprovação.

---

# 4. Criar rota dedicada para edição do telefone do encarregado

## Objetivo

Separar `telefone_encarregado` da rota genérica de dados pessoais do estudante, seguindo o padrão de rotas dedicadas já usado para contatos.

## Regra de negócio

Criar rota protegida por autenticação de estudante:

- `PUT /estudante/encarregado/telefone`

Payload JSON:

```json
{
  "telefone_encarregado": "923456789"
}
```

O backend deve:

1. identificar o estudante exclusivamente pelo token;
2. validar telefone nacional estrito com 9 dígitos, sem DDI, espaços, hífens, parênteses ou letras;
3. rejeitar valor vazio;
4. alterar apenas o telefone do encarregado;
5. resetar `telefone_encarregado_verificado` quando o valor realmente mudar;
6. não aceitar `email`, `telefone`, `nome`, BI, data de nascimento, `codigo_estudante`, `academia_id`, `codigo_academia` ou qualquer seletor de alvo no payload.

## Escopo obrigatório

### 4.1 Remoção da rota genérica

`PUT /estudante/dados-pessoais` deve rejeitar `telefone_encarregado` com erro `400`, código de validação compatível com `campo_nao_permitido`, e mensagem orientando o uso de `PUT /estudante/encarregado/telefone`.

### 4.2 Testes obrigatórios

1. estudante altera telefone do encarregado pela rota dedicada;
2. telefone com DDI, espaços, hífens, parênteses, letras ou vazio é rejeitado;
3. rota dedicada não aceita campos extras sensíveis nem seletores de alvo;
4. rota genérica rejeita `telefone_encarregado` sem mutação parcial;
5. alteração efetiva reseta `telefone_encarregado_verificado` na projeção e em rebuild.

---

# 5. Consultas de solicitações

## Objetivo

Permitir acompanhamento operacional das solicitações pendentes, aprovadas e reprovadas.

## Regra de negócio

Criar:

- `GET /estudante/solicitacoes-edicao` — estudante lista suas próprias solicitações, com filtro opcional por `status` e `campo`;
- `GET /academia/solicitacoes-edicao-estudante` — academia lista solicitações de estudantes vinculados, com filtro opcional por `status`, `campo` e `codigo_estudante`.

Ambas devem seguir paginação `limit`/`offset`, padrão 50 e teto 100.

## Testes obrigatórios

1. estudante lista apenas as próprias solicitações;
2. academia lista apenas solicitações de seus estudantes;
3. filtros por `status`, `campo` e `codigo_estudante` funcionam;
4. paginação respeita teto de 100 itens.

---

# 6. Atualização obrigatória da documentação

Atualizar `Documentação.md` e OpenAPI/Swagger com:

- nova entidade de solicitação de edição de dados sensíveis do estudante;
- eventos adicionados ao ledger;
- rotas de solicitação por campo;
- rotas de aprovação/reprovação por campo;
- rotas de consulta;
- rota dedicada para telefone do encarregado;
- regra de armazenamento temporário e deleção do documento após aprovação/reprovação;
- regra de rejeição dos campos sensíveis na rota genérica `PUT /estudante/dados-pessoais`.

---

# Fora de escopo

- Permitir que o estudante aprove sua própria solicitação.
- Permitir que Admin decida solicitações de dados sensíveis de estudante, salvo se uma nova tarefa definir esse fluxo.
- Alterar dados sensíveis sem documento comprovativo.
- Criar endpoint genérico que receba `campo` arbitrário para solicitar ou aprovar alterações.
- Manter compatibilidade com payloads antigos que editavam os campos sensíveis pela rota genérica.
- Notificações por email/push.

# Critérios de aceite

A tarefa só deve ser considerada concluída quando:

1. houver entidade/eventos auditáveis para solicitação de edição de dados sensíveis do estudante;
2. estudante conseguir solicitar alteração de `nome`, `bilhete_identidade`, `bilhete_identidade_encarregado` e `data_nascimento` por rotas dedicadas separadas;
3. cada solicitação exigir PDF comprovativo em storage temporário;
4. academia vinculada conseguir aprovar/reprovar cada campo por rota dedicada correspondente;
5. aprovação alterar somente o campo solicitado e registrar evento no ledger do estudante;
6. reprovação preservar o dado vigente;
7. documento temporário for deletado após aprovação e após reprovação;
8. `PUT /estudante/dados-pessoais` rejeitar os quatro campos sensíveis e `telefone_encarregado`;
9. `PUT /estudante/encarregado/telefone` existir e atualizar somente o telefone do encarregado;
10. testes automatizados cobrirem os cenários obrigatórios das seções 2.2, 3.5, 4.2 e 5;
11. `Documentação.md` e OpenAPI/Swagger estiverem atualizados;
12. o PR explicar claramente que não há aliases, wrappers ou rotas genéricas para estes campos.

## Procedimento de conclusão

Ao implementar esta tarefa, mover este arquivo para `docs/Tarefas feitas/`, remover o sufixo `(pendente)` do título, atualizar o frontmatter para `status: concluida`, registrar os testes executados e criar um documento de debug em `docs/Debbugs/` caso alguma lacuna seja encontrada durante a auditoria final.


## Testes executados

- `go test ./...`
