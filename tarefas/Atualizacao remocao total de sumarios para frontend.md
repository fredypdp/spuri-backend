---
modificado: 2026-06-29 00:00
criado: 2026-06-29 00:00
---
# Atualização da remoção total de sumários/aulas para o frontend

## Resumo rápido

A entidade **sumário/aula** foi removida do backend e deixou de fazer parte do contrato público da API. O frontend não deve mais exibir telas, formulários, chamadas, filtros ou vínculos relacionados a sumários.

Rotas removidas:

- `GET /academia/sumarios`
- `GET /academia/sumarios/:id`
- `POST /academia/sumarios`
- `PUT /academia/sumarios/:id`
- `DELETE /academia/sumarios/:id`

As faltas continuam existindo normalmente, mas agora são independentes e não aceitam nem retornam dados de sumário.

## O que mudou nas faltas

Os payloads de criação e atualização de faltas não devem enviar:

- `sumario_id`
- `sumario_titulo`
- qualquer campo equivalente usado para vincular falta a sumário/aula

As respostas de listagem, consulta, criação e atualização de faltas também não retornam mais:

- `sumario_id`
- `sumario_titulo`

Payload de criação esperado:

```json
{
  "codigo_estudante": "ABC1234",
  "data": "2025-03-15",
  "materia_disciplinar_id": "uuid",
  "quantidade": 2,
  "observacao": "string opcional"
}
```

Payload de atualização esperado:

```json
{
  "id": "uuid-da-falta",
  "data": "2025-03-16",
  "materia_disciplinar_id": "uuid",
  "quantidade": 3,
  "observacao": "justificativa obrigatória"
}
```

## Erros que o frontend deve tratar

| Status | Situação | Como o frontend deve agir |
| --- | --- | --- |
| `404` ou método/rota não encontrada | Chamada para `/academia/sumarios` ou `/academia/sumarios/:id` | Remover a chamada. Não tentar fallback, alias ou nova tentativa automática. |
| `400` | Envio de `sumario_id` em criação/atualização de falta | Remover o campo do payload e ajustar o formulário para não depender de sumário. |
| `400` | Envio de `sumario_titulo` em criação/atualização de falta | Remover o campo do payload. O backend não aceita snapshot histórico de sumário. |

Exemplo de erro esperado quando um campo legado for enviado em faltas:

```json
{
  "error": "VALIDATION_ERROR",
  "message": "campo não suportado em falta: sumario_id",
  "request_id": "uuid-da-requisicao"
}
```

## Recomendações para implementação no frontend

1. Remover telas e menus de cadastro, edição, listagem e detalhe de sumários/aulas.
2. Remover chamadas HTTP para `/academia/sumarios` e `/academia/sumarios/:id`.
3. Remover seletores de sumário dos formulários de lançamento e correção de faltas.
4. Remover `sumario_id` e `sumario_titulo` dos tipos/interfaces de falta.
5. Remover colunas, chips, badges ou textos que mostrem título de sumário em listas ou detalhes de faltas.
6. Não manter cache local, store, query key ou mock de sumários.
7. Ao tratar respostas de faltas, assumir que a falta contém apenas seus dados próprios e vínculos acadêmicos válidos: estudante, matéria, ano letivo, ano acadêmico, data, quantidade e observação.
8. Se algum usuário tentar acessar uma tela antiga por rota salva/favorito, redirecionar para a área de faltas ou para uma página informando que o recurso foi removido.

## Ajustes esperados por tela

### Tela de sumários/aulas

- Remover a tela do roteamento do frontend.
- Remover botões de criar, editar e excluir sumário.
- Remover serviços, hooks e queries dedicados a sumários.

### Formulário de criação de falta

- Não carregar lista de sumários.
- Não exibir campo de seleção de sumário.
- Enviar somente `codigo_estudante`, `data`, `materia_disciplinar_id`, `quantidade` e `observacao`, quando houver observação.

### Formulário de atualização de falta

- Não permitir alterar vínculo de sumário.
- Enviar `observacao` como justificativa obrigatória da correção.
- Enviar apenas os campos próprios da falta que forem alterados.

### Listagem e detalhe de faltas

- Remover coluna/campo de sumário.
- Não esperar `sumario_id` nem `sumario_titulo` na resposta.
- Manter a exibição dos dados de estudante, matéria, data, quantidade, ano letivo, ano acadêmico e observação.

## Observação importante

Não há compatibilidade retroativa para sumários/aulas. O frontend deve remover a integração, e não apenas esconder temporariamente a UI. Payloads antigos com vínculo de sumário devem ser considerados inválidos.
