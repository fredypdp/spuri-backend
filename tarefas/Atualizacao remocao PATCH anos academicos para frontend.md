---
modificado: 2026-06-29 00:00
criado: 2026-06-29 00:00
---
# Atualização da remoção do PATCH de anos acadêmicos para o frontend

## Resumo rápido

A rota **`PATCH /academia/anos-academicos`** foi removida do backend e deixou de fazer parte do contrato público da API. O frontend não deve mais usar substituição em massa de anos acadêmicos, períodos ou semestres.

Rotas mantidas para anos acadêmicos:

- `GET /academia/anos-academicos`
- `POST /academia/anos-academicos`
- `DELETE /academia/anos-academicos`

Rota removida:

- `PATCH /academia/anos-academicos`

O fluxo agora é estritamente incremental: `POST` adiciona anos acadêmicos permitidos e `DELETE` remove anos acadêmicos permitidos quando a remoção for segura.

## O que mudou

### Anos acadêmicos

- Não existe mais operação para substituir a lista inteira de anos acadêmicos.
- `POST /academia/anos-academicos` apenas adiciona itens ao estado atual.
- `DELETE /academia/anos-academicos` apenas remove itens do estado atual.
- Payloads com intenção de substituição, como `substituir`, `replace`, `patch`, `set` ou `update`, devem ser removidos do frontend.
- Escritas não devem enviar `codigo_academia`; o backend usa sempre a academia autenticada.

### Academia fundamental

Academias cadastradas como `nivel="escola"` e `nivel_escolar="fundamental"` agora podem adicionar e remover anos fundamentais válidos com `type="fundamental"`, respeitando as validações de segurança.

Esse cenário não deve mais receber erro de nível incompatível.

### Curso médio

Cursos médios devem manter anos em sequência contínua, começando sempre em `1_ano_medio`.

Exemplos válidos:

```json
["1_ano_medio"]
```

```json
["1_ano_medio", "2_ano_medio", "3_ano_medio"]
```

Exemplos inválidos:

```json
["2_ano_medio", "3_ano_medio"]
```

```json
["1_ano_medio", "3_ano_medio"]
```

### Curso superior

Cursos superiores não aceitam adição ou remoção direta de anos acadêmicos, períodos ou semestres por `/academia/anos-academicos`.

O frontend não deve enviar `type="superior"` para `POST` ou `DELETE /academia/anos-academicos` esperando alteração direta de períodos/semestres.

### Dados cadastrais do curso

A rota `PUT /academia/curso/:id/dados` ficou restrita a dados cadastrais e rejeita payloads que tentem alterar anos acadêmicos, períodos ou semestres.

Não enviar nessa rota:

- `anos_academicos`
- `anosAcademicos`
- `periodos`
- `semestres`
- `quantidade_semestres`
- `anos`

## Erros que o frontend deve tratar

| Status | Situação | Como o frontend deve agir |
| --- | --- | --- |
| `404` ou método/rota não encontrada | Chamada para `PATCH /academia/anos-academicos` | Remover a chamada. Não tentar fallback, alias ou nova tentativa automática. |
| `400` | Payload de anos acadêmicos com campos de substituição em massa | Remover campos como `substituir`, `replace`, `patch`, `set` e `update`. |
| `400` | Escrita de anos acadêmicos com `codigo_academia` | Remover `codigo_academia` do body; a escrita usa a academia autenticada. |
| `400` | Curso médio ficaria sem anos acadêmicos | Bloquear a ação na UI antes do envio. |
| `400` | Curso médio ficaria com lacuna na sequência | Exigir sequência contínua desde `1_ano_medio`. |
| `400` | `POST` ou `DELETE` com `type="superior"` | Ocultar/bloquear edição direta de períodos superiores nessa tela. |
| `400` | `PUT /academia/curso/:id/dados` com anos/períodos/semestres | Remover esses campos do formulário de dados cadastrais do curso. |
| `409` | Remoção de ano com estudantes ativos vinculados | Orientar transferência, conclusão ou inativação dos estudantes antes de tentar novamente. |

Exemplo de erro de substituição em massa:

```json
{
  "error": "VALIDATION_ERROR",
  "message": "Payloads de substituição em massa não são suportados. Use POST para adicionar ou DELETE para remover anos acadêmicos.",
  "request_id": "uuid-da-requisicao",
  "details": [
    {
      "field": "replace",
      "code": "campo_nao_permitido",
      "message": "Payloads de substituição em massa não são suportados. Use POST para adicionar ou DELETE para remover anos acadêmicos."
    }
  ]
}
```

## Recomendações para implementação no frontend

1. Remover serviços, hooks, mutations e testes que chamem `PATCH /academia/anos-academicos`.
2. Separar claramente ações de adicionar e remover anos acadêmicos.
3. Não montar payloads de substituição completa da lista.
4. Validar no formulário de médio que a lista final permanece sequencial desde `1_ano_medio`.
5. Ocultar ações de escrita direta de períodos/semestres superiores em `/academia/anos-academicos`.
6. Remover campos acadêmicos do formulário de edição cadastral do curso (`PUT /academia/curso/:id/dados`).
7. Manter leitura com `GET /academia/anos-academicos` para montar a tela e descobrir os anos já configurados.
8. Usar `details[0].field`, `details[0].code` e `details[0].message` para destacar o campo e exibir mensagem específica ao usuário.

## Ajustes esperados por tela

### Tela de anos acadêmicos

- Manter consulta por `GET /academia/anos-academicos`.
- Trocar qualquer ação de salvar lista completa por ações explícitas de adicionar/remover.
- Remover botão ou fluxo de “substituir todos”.

### Formulário de fundamental

- Enviar `POST` para adicionar anos fundamentais.
- Enviar `DELETE` para remover anos fundamentais.
- Não bloquear academia `nivel="escola"` e `nivel_escolar="fundamental"` ao usar `type="fundamental"`.

### Formulário de médio

- Exigir `curso_id`.
- Enviar apenas anos no formato `[n]_ano_medio`.
- Impedir lacunas e remoção de todos os anos do curso.

### Formulário de superior

- Não enviar `POST` ou `DELETE /academia/anos-academicos` para alterar períodos/semestres diretamente.
- Não enviar anos acadêmicos, períodos ou semestres por `PUT /academia/curso/:id/dados`.

### Edição cadastral do curso

- Enviar apenas campos cadastrais permitidos, como `nome`.
- Remover campos de anos acadêmicos, períodos e semestres desse fluxo.

## Observação importante

Não há compatibilidade retroativa para `PATCH /academia/anos-academicos`. O frontend deve remover a integração e migrar para operações explícitas de adição e remoção segura.
