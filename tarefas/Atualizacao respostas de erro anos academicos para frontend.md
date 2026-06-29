---
modificado: 2026-06-29 00:00
criado: 2026-06-29 00:00
---
# Atualização das respostas de erro de anos acadêmicos para o frontend

## Resumo rápido

As rotas de anos acadêmicos passaram a retornar erros mais claros e estruturados para que o frontend consiga mostrar ao usuário exatamente qual campo está errado e orientar a correção.

Rotas afetadas:

- `GET /academia/anos-academicos`
- `POST /academia/anos-academicos`
- `PATCH /academia/anos-academicos`
- `DELETE /academia/anos-academicos`

A estrutura de sucesso das rotas não mudou. A mudança relevante está nas respostas de erro de validação (`400`) e conflito (`409`).

## O que mudou na resposta de erro

Além de `error`, `message` e `request_id`, os erros dessas rotas agora podem trazer `details`, com um item indicando:

- `field`: campo que precisa ser corrigido;
- `code`: código interno do tipo de erro;
- `message`: explicação completa para exibir ao usuário ou usar no debug.

Exemplo:

```json
{
  "error": "VALIDATION_ERROR",
  "message": "O campo 'curso_id' é obrigatório quando type='medio', porque anos de médio/superior pertencem a um curso específico.",
  "request_id": "uuid-da-requisicao",
  "details": [
    {
      "field": "curso_id",
      "code": "campo_obrigatorio",
      "message": "O campo 'curso_id' é obrigatório quando type='medio', porque anos de médio/superior pertencem a um curso específico."
    }
  ]
}
```

## Códigos que o frontend deve tratar

| Status | `field` | `code` | Como o frontend deve agir |
| --- | --- | --- | --- |
| `400` | `payload` | `json_invalido` | Mostrar erro geral informando que a requisição está mal formada. Normalmente indica bug no client ou payload montado incorretamente. |
| `400` | `type` | `valor_invalido` | Destacar o seletor/campo de tipo e permitir apenas `fundamental`, `medio` ou `superior`. |
| `400` | `type` | `nivel_incompativel` | Bloquear/ocultar ação de fundamental para academia que não é escolar fundamental/mista. |
| `400` | `type` | `tipo_diferente_do_curso` | Conferir se o curso selecionado corresponde ao tipo escolhido no formulário. |
| `400` | `codigo_academia` | `campo_obrigatorio` | Em telas/admin, enviar `codigo_academia` na query ao consultar anos acadêmicos. |
| `400` | `curso_id` | `campo_obrigatorio` | Exigir seleção de curso quando `type` for `medio` ou `superior`. |
| `400` | `curso_id` | `nao_encontrado` | Recarregar lista de cursos e informar que o curso selecionado não existe mais ou o ID está incorreto. |
| `400` | `curso_id` | `curso_de_outra_academia` | Impedir uso de curso que não pertence à academia atual; recarregar cursos da academia autenticada. |
| `400` | `curso_id` | `curso_inativo` | Informar que somente cursos ativos podem ter anos/períodos editados. |
| `400` | `anos_academicos` | `campo_obrigatorio` | Exigir pelo menos um ano para `fundamental` ou `medio`. |
| `400` | `anos_academicos` | `formato_invalido` | Mostrar formatos aceitos: `1_ano_fundamental` a `9_ano_fundamental` ou `[n]_ano_medio`. |
| `400` | `anos_academicos` | `remocao_invalida` | Impedir operação que deixa academia fundamental/mista sem nenhum ano ativo. |
| `400` | `anos_academicos` | `campo_nao_permitido` | Para curso superior, não enviar `anos_academicos`; enviar apenas `periodos`. |
| `400` | `periodos` | `campo_obrigatorio` | Para curso superior, exigir a quantidade total de semestres. |
| `400` | `periodos` | `valor_invalido` | Validar que `periodos` é número inteiro positivo antes de enviar. |
| `409` | `anos_academicos` | `estudantes_ativos_vinculados` | Bloquear confirmação simples e orientar o usuário a transferir, concluir ou inativar estudantes antes de remover/reduzir anos ou períodos. |

## Recomendações para implementação no frontend

1. Ler sempre `response.details?.[0]` antes de cair em mensagens genéricas.
2. Usar `details[0].field` para destacar o campo do formulário com erro.
3. Usar `details[0].message` como mensagem principal ao usuário, pois ela já vem em linguagem mais explicativa.
4. Usar `details[0].code` para regras específicas de UI, como ocultar campos ou bloquear ações.
5. Manter fallback para `response.message` quando `details` não vier preenchido, porque outras rotas ainda podem usar apenas o envelope padrão.
6. Registrar `request_id` nos logs do frontend/suporte para facilitar debug com o backend.

## Ajustes esperados por tela

### Listagem/consulta de anos acadêmicos

- Se usuário for admin, garantir que a query `codigo_academia` seja enviada.
- Em erro `codigo_academia/campo_obrigatorio`, pedir seleção de uma academia antes de consultar.

### Formulário de fundamental

- Enviar `type="fundamental"` e `anos_academicos`.
- Não permitir lista vazia quando o objetivo for substituir/remover todos os anos.
- Mostrar erro de `nivel_incompativel` quando a academia não puder gerenciar fundamental.

### Formulário de médio

- Exigir curso médio selecionado (`curso_id`).
- Enviar `anos_academicos` no formato `[n]_ano_medio`.
- Se houver `curso_inativo`, orientar o usuário a ativar o curso antes da edição.

### Formulário de superior

- Exigir curso superior selecionado (`curso_id`).
- Enviar somente `periodos` como número inteiro positivo.
- Não enviar `anos_academicos`; o backend calcula automaticamente os anos superiores.

### Remoção/redução de anos ou períodos

- Se o backend retornar `409` com `estudantes_ativos_vinculados`, informar que existem estudantes ativos vinculados.
- Não tentar reenviar automaticamente a mesma requisição.
- Orientar a academia a transferir, concluir ou inativar os estudantes antes de tentar novamente.

## Exemplo de tratamento genérico

```ts
const detail = response.details?.[0];

if (detail) {
  setFieldError(detail.field, detail.message);
  showToast(detail.message);

  if (detail.code === "estudantes_ativos_vinculados") {
    openBlockingStudentsGuidance();
  }
} else {
  showToast(response.message || "Não foi possível concluir a operação.");
}

if (response.request_id) {
  logSupportRequestId(response.request_id);
}
```

## Observação importante

Esta atualização não exige mudança nos payloads de sucesso nem nas respostas `200`. O frontend deve apenas melhorar o tratamento de erros para aproveitar `details[]` e evitar mensagens vagas para usuários leigos.
