# API INTELIZE PAGAMENTOS - Documentação consolidada

Documentação extraída da coleção Postman **"API INTELIZE PAGAMENTOS"** (`_postman_id: 939ba24c-227e-45d5-830f-76394690bc7d`), exportada em formato Collection v2.1.

## Sumário

1. [Intro](#intro)
2. [Autenticação](#autenticação)
3. [Variáveis de ambiente](#variáveis-de-ambiente)
4. [Formatos de resposta](#formatos-de-resposta)
5. [Referências de pagamentos](#referências-de-pagamentos)
   - [Pegar todas as referências](#pegar-todas-as-referências)
   - [Obter referência por número de referência](#obter-referência-por-número-de-referência)
   - [Obter referência por ID](#obter-referência-por-id)
   - [Obter referências por intervalo de dias](#obter-referências-por-intervalo-de-dias)
   - [Gerar referências](#gerar-referências)
   - [Actualizar referências](#actualizar-referências)
   - [Deletar referências](#deletar-referências)
6. [Pagamentos](#pagamentos)
   - [Todos os pagamentos](#todos-os-pagamentos)
   - [Pagamento por referência](#pagamento-por-referência)
   - [Pagamento pelo ID](#pagamento-pelo-id)
   - [Pagamentos por intervalo de datas](#pagamentos-por-intervalo-de-datas)
7. [Dicionário de campos](#dicionário-de-campos)
8. [Inconsistências detectadas na coleção original](#inconsistências-detectadas-na-coleção-original)

---

## Intro

**A API INTELIZE PAGAMENTOS** permite aceitar diversas formas de pagamento nos terminais **ATM**, **Multicaixa Express** e **Internet Bank**, usando uma única API, integrando-se com a solução ou software da empresa. Um objecto da INTELIZE PAGAMENTOS contém os dados da forma de pagamento para criar **referências**, e estas são creditadas e visualizadas na plataforma da Intelize.

A API está organizada em dois grandes grupos de recursos:

- **Referências de pagamentos** — criação, consulta, actualização e exclusão de referências de pagamento (as referências são o mecanismo central da plataforma; é através delas que os pagamentos são efectivamente realizados nos terminais).
- **Pagamentos** — consulta de pagamentos já efectuados/conciliados sobre as referências criadas.

---

## Autenticação

**Tipo:** Bearer Token

Todos os endpoints desta API exigem um cabeçalho de autorização:

```
Authorization: Bearer <token>
```

> ⚠️ **Nota de segurança:** o token de acesso é uma credencial sensível, associada à conta/entidade do comerciante (inclui NIF, contacto e email da entidade). Nunca deve ser exposto em repositórios de código, documentação partilhada, logs, ou coleções Postman exportadas/commitadas. Deve ser armazenado apenas em variáveis de ambiente seguras (backend) e rodado periodicamente.

Não há, na coleção original, um endpoint documentado de emissão/renovação de token (`Get a token`) — o token parece ser emitido/gerido directamente pela Intelize por fora da API (ex: painel administrativo ou processo manual de onboarding). **Recomenda-se confirmar com a Intelize** qual é o mecanismo oficial de obtenção/renovação deste Bearer Token e o seu tempo de expiração, já que isto não está documentado na coleção.

---

## Variáveis de ambiente

| Variável | Valor de exemplo (dev) | Descrição |
| --- | --- | --- |
| `HOST_API` | `http://127.0.0.1:8080/v1` | URL base da API. Deve ser substituída pelo endpoint de produção/homologação fornecido pela Intelize. |

Todos os endpoints abaixo usam `{{HOST_API}}` como prefixo.

---

## Formatos de resposta

A maior parte dos endpoints `GET` desta API suporta alternar o formato da resposta através do parâmetro de query **`formato`**:

| Valor de `formato` | Content-Type da resposta | Observação |
| --- | --- | --- |
| *(omitido)* | `application/json; charset=utf-8` | Formato por omissão |
| `xml` | `text/html; charset=utf-8` | Corpo da resposta em XML, mas servido com `Content-Type: text/html` |
| `csv` | `text/html; charset=utf-8` | Corpo da resposta em CSV (com aspas duplas escapadas), também servido com `Content-Type: text/html` |

> **Nota:** tanto no formato XML como no CSV, o cabeçalho `Content-Type` devolvido pelo servidor é `text/html`, não `application/xml` ou `text/csv`. Isto é uma particularidade da implementação actual da API e deve ser tido em conta ao integrar (não confiar apenas no `Content-Type` para decidir como fazer parsing — usar o parâmetro `formato` que foi enviado no pedido).

Exemplo de uso:
```
GET {{HOST_API}}/auth/referencias?formato=xml
GET {{HOST_API}}/auth/referencias?formato=csv
```

---

## Referências de pagamentos

> As referências são uma das partes mais importantes da plataforma — é através delas que se concretiza a realização de pagamentos. Esta secção cobre como **gerar**, **actualizar** e **excluir** referências, bem como consultá-las.

### Pegar todas as referências

`GET {{HOST_API}}/auth/referencias/`

Busca todas as referências geradas pela entidade autenticada, incluindo activas e inactivas.

**Headers**
| Nome | Obrigatório | Descrição |
| --- | --- | --- |
| `Authorization` | Sim | `Bearer <token>` |

**Query Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `formato` | string | Não | `xml` ou `csv`. Omitir para JSON (padrão). |

**Resposta de exemplo — `200 OK` (JSON)**
```json
{
    "status": "sucesso",
    "mensagem": [
        {
            "id_referencia": 79,
            "entidade_cliente": "00987",
            "criada_r": "2023-07-21T15:30:00.000Z",
            "num_referencia": "949584361",
            "data_limite_pagamento": "2023-10-07",
            "indicador_de_produtos": "1",
            "tipo_de_registro": "1",
            "referencia_do_montante": null,
            "codigo_de_processamento": "80",
            "textos_para_talao": null,
            "quantidade_de_unidades": null,
            "codigo_de_ativacao": null,
            "numero_serie_helpDesk": null,
            "chave_ativacao": null,
            "data_de_validade": null,
            "montante_maximo": null,
            "data_inicio_de_pagamento": null,
            "montante_minimo": null,
            "codigo_de_cliente": null,
            "numero_de_linhas": null,
            "actualiza_em": "2023-07-21T15:30:00.000Z",
            "indicador_produto_id": "6",
            "id_tipo_produto": 1,
            "registo_produto": "Pagamento/Carregamento",
            "id_produto": 6,
            "cliente_tipo_produto": 7,
            "criado_quando": "2023-07-19T15:08:14.000Z",
            "produto": "Paga Só Pagamentos",
            "codigo_do_produto": 1
        }
    ]
}
```
*(a resposta real devolve um array com todas as referências da entidade; o exemplo acima foi reduzido a um item para leitura — ver [Dicionário de campos](#dicionário-de-campos) para o significado de cada campo)*

Campos como `mensagem` são sempre um **array**, mesmo quando devolvem uma única referência.

---

### Obter referência por número de referência

`GET {{HOST_API}}/auth/referencias/referencia/{num_referencia}`

Filtra as referências da entidade e devolve uma pelo seu **código de referência** (`num_referencia`).

**Path Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `num_referencia` | string | Sim | Número da referência a consultar. Exemplo: `934489103` |

**Query Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `formato` | string | Não | `xml` ou `csv`. Omitir para JSON (padrão). |

**Resposta de exemplo — `200 OK` (JSON)**
```json
{
    "status": "sucesso",
    "mensagem": [
        {
            "id_referencia": 78,
            "entidade_cliente": "00987",
            "criada_r": "2023-07-19T15:39:15.000Z",
            "num_referencia": "934489103",
            "data_limite_pagamento": "2024-07-19",
            "indicador_de_produtos": "1",
            "tipo_de_registro": "1",
            "codigo_de_processamento": "80",
            "actualiza_em": "2023-07-19T15:39:15.000Z",
            "indicador_produto_id": "6",
            "id_tipo_produto": 1,
            "registo_produto": "Pagamento/Carregamento",
            "id_produto": 6,
            "cliente_tipo_produto": 7,
            "criado_quando": "2023-07-19T15:08:14.000Z",
            "produto": "Paga Só Pagamentos",
            "codigo_do_produto": 1
        }
    ]
}
```

---

### Obter referência por ID

`GET {{HOST_API}}/auth/referencias/{id_referencia}`

Filtra as referências da entidade e devolve uma pelo seu **ID interno** (`id_referencia`), diferente do `num_referencia`.

> ⚠️ Ver [Inconsistências detectadas](#inconsistências-detectadas-na-coleção-original) — o campo de URL deste endpoint estava vazio na coleção exportada; a estrutura abaixo foi reconstruída a partir do exemplo de resposta gravado.

**Path Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `id_referencia` | integer | Sim | ID interno da referência. Exemplo: `78` |

**Query Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `formato` | string | Não | `xml` ou `csv`. Omitir para JSON (padrão). |

**Resposta de exemplo — `200 OK` (JSON)**
```json
{
    "status": "sucesso",
    "mensagem": [
        {
            "id_referencia": 78,
            "entidade_cliente": "00987",
            "num_referencia": "934489103",
            "data_limite_pagamento": "2024-07-19",
            "indicador_de_produtos": "1",
            "tipo_de_registro": "1",
            "codigo_de_processamento": "80",
            "produto": "Paga Só Pagamentos",
            "codigo_do_produto": 1
        }
    ]
}
```

---

### Obter referências por intervalo de dias

`GET {{HOST_API}}/auth/referencias/dia-inicio/{data_inicio}/dia-final/{data_final}`

Devolve todas as referências criadas dentro de um intervalo de datas.

> ⚠️ Ver [Inconsistências detectadas](#inconsistências-detectadas-na-coleção-original) — o campo de URL deste endpoint também estava vazio na coleção exportada; reconstruído a partir do exemplo de resposta gravado.

**Path Parameters**
| Nome | Tipo | Obrigatório | Descrição | Formato | Exemplo |
| --- | --- | --- | --- | --- | --- |
| `data_inicio` | string\<date\> | Sim | Data inicial do intervalo | `YYYY-MM-DD` | `2023-01-01` |
| `data_final` | string\<date\> | Sim | Data final do intervalo | `YYYY-MM-DD` | `2023-07-22` |

**Query Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `formato` | string | Não | `xml` ou `csv`. Omitir para JSON (padrão). |

**Resposta de exemplo — `200 OK` (JSON)**
```json
{
    "status": "sucesso",
    "mensagem": [
        {
            "id_referencia": 79,
            "entidade_cliente": "00987",
            "num_referencia": "949584361",
            "data_limite_pagamento": "2023-10-07",
            "tipo_de_registro": "1",
            "produto": "Paga Só Pagamentos"
        },
        {
            "id_referencia": 78,
            "entidade_cliente": "00987",
            "num_referencia": "934489103",
            "data_limite_pagamento": "2024-07-19",
            "tipo_de_registro": "1",
            "produto": "Paga Só Pagamentos"
        }
    ]
}
```

---

### Gerar referências

`POST {{HOST_API}}/auth/referencias`

Gera uma nova referência de pagamento, com validade e ordem de serviço definidas com base no tipo de negócio.

> O corpo do pedido **varia consoante o `tipo_de_registro`** escolhido. Existem 3 variantes documentadas na coleção original — cada uma serve um propósito de negócio diferente (pagamentos/carregamentos avulsos, recargas com controlo de activação, ou facturas com prazo e limites de montante).

**Headers**
| Nome | Obrigatório | Descrição |
| --- | --- | --- |
| `Authorization` | Sim | `Bearer <token>` |
| `Content-Type` | Sim | `application/json` |

#### Variante 1 — Pagamentos ou Carregamentos (`tipo_de_registro: 1`)

**Body**
```json
{
    "tipo_de_registro": 1,
    "num_referencia": 947557527,
    "indicador_de_produtos": 1,
    "data_limite_pagamento": "2023-07-22",
    "indicador_produto_id": "1"
}
```

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `tipo_de_registro` | integer | Sim | `1` para pagamentos/carregamentos |
| `num_referencia` | integer | Sim | Número da referência a criar |
| `indicador_de_produtos` | integer | Sim | Indicador do produto associado |
| `data_limite_pagamento` | string\<date\> | Sim | Data limite para o pagamento (`YYYY-MM-DD`) |
| `indicador_produto_id` | string | Sim | ID do indicador do produto |

**Resposta — `200 OK`**
```json
{
    "status": "sucesso",
    "mensagem": "Referecia gerada com sucesso",
    "info": {
        "entidade_cliente": "00987",
        "tipo_de_registro": "1",
        "codigo_de_processamento": 80,
        "num_referencia": "947557527",
        "indicador_de_produtos": "1",
        "data_limite_pagamento": "2023-07-22",
        "indicador_produto_id": "1"
    }
}
```

#### Variante 2 — Recargas (`tipo_de_registro: 2`)

**Body**
```json
{
    "tipo_de_registro": 2,
    "indicador_de_produtos": 1,
    "referencia_do_montante": "93448",
    "quantidade_de_unidades": 1,
    "codigo_de_cliente": "INTELIZE-0001",
    "codigo_de_ativacao": "INTELIZE-COD",
    "numero_serie_helpDesk": "12345",
    "chave_ativacao": "12345",
    "data_de_validade": "2023-09-22",
    "indicador_produto_id": "1"
}
```

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `tipo_de_registro` | integer | Sim | `2` para recargas |
| `indicador_de_produtos` | integer | Sim | Indicador do produto associado |
| `referencia_do_montante` | string | Não | Referência associada ao montante da recarga |
| `quantidade_de_unidades` | integer | Não | Quantidade de unidades a recarregar |
| `codigo_de_cliente` | string | Não | Código do cliente na Intelize |
| `codigo_de_ativacao` | string | Não | Código de activação da recarga |
| `numero_serie_helpDesk` | string | Não | Número de série para suporte/HelpDesk |
| `chave_ativacao` | string | Não | Chave de activação |
| `data_de_validade` | string\<date\> | Não | Data de validade da recarga (`YYYY-MM-DD`) |
| `indicador_produto_id` | string | Sim | ID do indicador do produto |

**Resposta — `200 OK`**
```json
{
    "status": "sucesso",
    "mensagem": "Referecia gerada com sucesso",
    "info": {
        "entidade_cliente": "01157",
        "tipo_de_registro": "2",
        "codigo_de_processamento": 80,
        "referencia_do_montante": "93448",
        "indicador_de_produtos": "1",
        "quantidade_de_unidades": "1",
        "codigo_de_cliente": "INTELIZE-0001",
        "codigo_de_ativacao": "INTELIZE-COD",
        "numero_serie_helpDesk": "12345",
        "chave_ativacao": "12345",
        "data_de_validade": "2023-09-22",
        "indicador_produto_id": "1"
    }
}
```

#### Variante 3 — Facturas (`tipo_de_registro: 3`)

**Body**
```json
{
    "tipo_de_registro": 3,
    "indicador_de_produtos": 1,
    "codigo_de_cliente": "INTELIZE-0001",
    "num_referencia": "123456789",
    "data_limite_pagamento": "2023-09-09",
    "data_inicio_de_pagamento": "2033-08-19",
    "montante_minimo": "1000000",
    "montante_maximo": "999999",
    "numero_de_linhas": 1,
    "textos_para_talao": "MUITO OBRIGADO",
    "indicador_produto_id": 1
}
```

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `tipo_de_registro` | integer | Sim | `3` para facturas |
| `indicador_de_produtos` | integer | Sim | Indicador do produto associado |
| `codigo_de_cliente` | string | Não | Código do cliente na Intelize |
| `num_referencia` | string | Sim | Número da referência a criar |
| `data_limite_pagamento` | string\<date\> | Sim | Data limite para o pagamento |
| `data_inicio_de_pagamento` | string\<date\> | Não | Data a partir da qual o pagamento é aceite |
| `montante_minimo` | string | Não | Montante mínimo aceite para a factura |
| `montante_maximo` | string | Não | Montante máximo aceite para a factura |
| `numero_de_linhas` | integer | Não | Número de linhas/parcelas da factura |
| `textos_para_talao` | string | Não | Texto livre impresso no talão/recibo |
| `indicador_produto_id` | integer | Sim | ID do indicador do produto |

**Resposta — `200 OK`**
```json
{
    "status": "sucesso",
    "mensagem": "Referecia gerada com sucesso",
    "info": {
        "entidade_cliente": "01157",
        "tipo_de_registro": "3",
        "codigo_de_processamento": 80,
        "num_referencia": "123456789",
        "indicador_de_produtos": "1",
        "data_limite_pagamento": "2023-09-09",
        "data_inicio_de_pagamento": "2033-08-19",
        "montante_minimo": "1000000",
        "montante_maximo": "999999",
        "codigo_de_cliente": "INTELIZE-0001",
        "numero_de_linhas": "1",
        "textos_para_talao": "MUITO OBRIGADO",
        "indicador_produto_id": "1"
    }
}
```

> **Nota:** em todas as variantes, `codigo_de_processamento` é devolvido pela API (não é enviado pelo cliente) — parece ser atribuído automaticamente conforme o tipo/produto (valores observados: `80`, `82`).

---

### Actualizar referências

`PATCH {{HOST_API}}/auth/referencias/{id_referencia}`

Permite actualizar uma referência já criada. As referências actualizadas são reenviadas para os terminais depois de actualizadas.

**Path Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `id_referencia` | integer | Sim | ID interno da referência a actualizar. Exemplo: `48` |

**Headers**
| Nome | Obrigatório | Descrição |
| --- | --- | --- |
| `Authorization` | Sim | `Bearer <token>` |
| `Content-Type` | Sim | `application/json` |

**Body** *(exemplo — envia apenas os campos a alterar, tipo PATCH parcial)*
```json
{
    "numero_de_linhas": 2
}
```

**Resposta — `200 OK`**
```json
{
    "status": "sucesso",
    "mensagem": "Referecia actualizada com sucesso",
    "info": {
        "antes": [
            {
                "id_referencia": 48,
                "entidade_cliente": "01157",
                "num_referencia": "123456789",
                "data_limite_pagamento": "2023-09-09",
                "numero_de_linhas": "3",
                "actualiza_em": "2023-07-23T22:28:35.000Z"
            }
        ],
        "depois": [
            {
                "id_referencia": 48,
                "entidade_cliente": "01157",
                "num_referencia": "123456789",
                "data_limite_pagamento": "2023-09-09",
                "numero_de_linhas": "2",
                "actualiza_em": "2023-07-23T22:28:47.000Z"
            }
        ]
    }
}
```

A resposta devolve o estado **`antes`** e **`depois`** da actualização, permitindo confirmar exactamente o que mudou (útil para auditoria).

---

### Deletar referências

`DELETE {{HOST_API}}/auth/referencias/{id_referencia}`

Exclui uma referência. **Importante:** a referência deve estar inactiva ou desvinculada dos terminais antes da exclusão. Recomenda-se definir sempre um tempo de validade ao gerar a referência, para reduzir a necessidade de exclusões manuais.

**Path Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `id_referencia` | integer | Sim | ID interno da referência a excluir. Exemplo: `48` |

**Headers**
| Nome | Obrigatório | Descrição |
| --- | --- | --- |
| `Authorization` | Sim | `Bearer <token>` |

**Resposta — `200 OK`**
```json
{
    "status": "sucesso",
    "mensagem": "Referecia excluida com sucesso",
    "info": {
        "id_referencia": 48,
        "entidade_cliente": "01157",
        "num_referencia": "123456789",
        "data_limite_pagamento": "2023-09-09",
        "tipo_de_registro": "3",
        "textos_para_talao": "MUITO OBRIGADO",
        "montante_maximo": "999999",
        "montante_minimo": "1000000",
        "codigo_de_cliente": "INTELIZE-0001",
        "numero_de_linhas": "2",
        "actualiza_em": "2023-07-23T22:28:47.000Z",
        "indicador_produto_id": "1"
    }
}
```

> ⚠️ A coleção original tinha um `body` de exemplo (`{"numero_de_linhas": 2}`) associado a este pedido `DELETE`. Isto é atípico — pedidos `DELETE` normalmente não têm corpo. É provável que este body tenha sido copiado por engano do endpoint `PATCH` acima durante a duplicação do pedido no Postman, e não seja realmente exigido pela API. **Confirmar com a Intelize se o `DELETE` exige ou ignora corpo de pedido.**

---

## Pagamentos

> Esta secção permite consultar os pagamentos feitos à entidade — em tempo real, com comprovativos, e também os já conciliados.

### Todos os pagamentos

`GET {{HOST_API}}/auth/pagamentos`

Devolve todos os pagamentos recebidos pela entidade.

**Query Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `formato` | string | Não | `xml` ou `csv`. Omitir para JSON (padrão). |

**Resposta de exemplo — `200 OK` (JSON, reduzida a 1 item)**
```json
{
    "status": "sucesso",
    "mensagem": [
        {
            "id_pagamento": 22,
            "montante_da_operacao": "13200.69",
            "montante_total_transicoes": "00000000003319214",
            "data_do_processamento": "2023-06-07",
            "id_ultimo_ficheiro": "20230607001",
            "tarifa_aplicada_a_operacao": "000000015000",
            "total_transicao": "000000060000",
            "referencia_do_servico": "747190407",
            "numero_entidade": "01157",
            "data_hora_transacao_cliente": "2023-06-07 16:19:17",
            "codigo_produto": "1",
            "quantidade_de_unidades": null,
            "operacao_Pagamento": "0",
            "localidade": "Internet       ",
            "modo_de_Envio_Comunicacao": "1",
            "codigo_de_Resposta_da_Empresa": "0",
            "n_de_Identificacao_da_Resposta": "059679870125",
            "PRT": "ACEITE",
            "MFT": "CONCLUIDO",
            "aplicaccao_PDD": "8",
            "Identificacao_Log_EGR": "3144",
            "numero_Log_EGR": "00000273",
            "codigo_parametro": "00",
            "data_movimento": "2",
            "hora_do_movimento": "0",
            "tipo_de_Terminal": "M",
            "numero_Periodo_Contabilistico": "000",
            "identificacao_Transacao_Local": "00000",
            "identificacao_do_Terminal": "0000000000",
            "RFU": "000000000000",
            "codigo_de_Moeda": "024",
            "id_referencia": 57,
            "entidade_cliente": "01157",
            "criada_r": "2023-06-07T15:01:00.000Z",
            "num_referencia": "747190407",
            "data_limite_pagamento": "2023-06-24",
            "indicador_de_produtos": "2",
            "tipo_de_registro": "1",
            "codigo_de_processamento": "82",
            "actualiza_em": "2023-06-20T12:50:11.000Z",
            "indicador_produto_id": "2",
            "id_produto": 2,
            "cliente_tipo_produto": 1,
            "criado_quando": "2023-05-25T16:15:07.000Z",
            "produto": "PAYKWANZA",
            "codigo_do_produto": 2,
            "id_tipo_produto": 1,
            "registo_produto": "Pagamento/Carregamento"
        }
    ]
}
```

Cada objecto de pagamento vem **fundido com os dados da referência associada** (campos como `entidade_cliente`, `num_referencia`, `produto`, etc. aparecem juntamente com os campos específicos da transacção, tipo `PRT`, `MFT`, `codigo_de_Moeda`). Ver [Dicionário de campos](#dicionário-de-campos).

---

### Pagamento por referência

`GET {{HOST_API}}/auth/pagamentos/referencia/{num_referencia}`

Devolve o(s) pagamento(s) associado(s) a um número de referência específico.

**Path Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `num_referencia` | string | Sim | Número da referência. Exemplo: `747190407` |

**Query Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `formato` | string | Não | `xml` ou `csv`. Omitir para JSON (padrão). |

**Resposta — `200 OK`**: mesmo formato do objecto de pagamento descrito acima, filtrado por `num_referencia`.

---

### Pagamento pelo ID

`GET {{HOST_API}}/auth/pagamentos/{id_pagamento}`

Devolve um pagamento específico pelo seu **ID interno** (`id_pagamento`).

> ⚠️ Ver [Inconsistências detectadas](#inconsistências-detectadas-na-coleção-original) — o pedido principal gravado na coleção apontava, por engano, para o mesmo URL do endpoint "Pagamento por referência". A estrutura abaixo foi reconstruída a partir do exemplo de resposta gravado, que usa efectivamente um ID (`22`).

**Path Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `id_pagamento` | integer | Sim | ID interno do pagamento. Exemplo: `22` |

**Query Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `formato` | string | Não | `xml` ou `csv`. Omitir para JSON (padrão). |

**Resposta — `200 OK`**: mesmo formato do objecto de pagamento descrito acima, filtrado por `id_pagamento`.

---

### Pagamentos por intervalo de datas

`GET {{HOST_API}}/auth/pagamentos/dia-inicio/{data_inicio}/dia-final/{data_final}`

Devolve todos os pagamentos processados dentro de um intervalo de datas.

**Path Parameters**
| Nome | Tipo | Obrigatório | Descrição | Formato | Exemplo |
| --- | --- | --- | --- | --- | --- |
| `data_inicio` | string\<date\> | Sim | Data inicial do intervalo | `YYYY-MM-DD` | `2023-01-01` |
| `data_final` | string\<date\> | Sim | Data final do intervalo | `YYYY-MM-DD` | `2023-08-10` |

**Query Parameters**
| Nome | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `formato` | string | Não | `xml` ou `csv`. Omitir para JSON (padrão). |

**Resposta — `200 OK`**: array de objectos de pagamento (mesmo formato descrito em "Todos os pagamentos"), filtrado pelo intervalo de datas.

---

## Dicionário de campos

Consolidado a partir de todos os exemplos de resposta observados. Nem todos os campos aparecem em todos os endpoints — campos ausentes tipicamente vêm como `null`.

### Referência

| Campo | Tipo | Descrição |
| --- | --- | --- |
| `id_referencia` | integer | ID interno da referência |
| `entidade_cliente` | string | Número de entidade do cliente/comerciante (5 dígitos) |
| `criada_r` | string\<date-time\> | Data/hora de criação da referência |
| `num_referencia` | string | Número da referência (usado nos terminais/ATM/Multicaixa) |
| `data_limite_pagamento` | string\<date\> | Data limite para efectuar o pagamento |
| `indicador_de_produtos` | string | Indicador do produto associado |
| `tipo_de_registro` | string | `1` = Pagamentos/Carregamentos, `2` = Recargas, `3` = Facturas |
| `referencia_do_montante` | string \| null | Referência associada ao montante (recargas) |
| `codigo_de_processamento` | string | Código de processamento atribuído pela API (ex: `80`, `82`) |
| `textos_para_talao` | string \| null | Texto livre impresso no talão/recibo |
| `quantidade_de_unidades` | string \| null | Quantidade de unidades (recargas) |
| `codigo_de_ativacao` | string \| null | Código de activação (recargas) |
| `numero_serie_helpDesk` | string \| null | Número de série para suporte |
| `chave_ativacao` | string \| null | Chave de activação (recargas) |
| `data_de_validade` | string\<date\> \| null | Data de validade (recargas) |
| `montante_maximo` | string \| null | Montante máximo aceite (facturas) |
| `data_inicio_de_pagamento` | string\<date\> \| null | Data a partir da qual o pagamento é aceite (facturas) |
| `montante_minimo` | string \| null | Montante mínimo aceite (facturas) |
| `codigo_de_cliente` | string \| null | Código do cliente na Intelize |
| `numero_de_linhas` | string \| null | Número de linhas/parcelas (facturas) |
| `actualiza_em` | string\<date-time\> | Data/hora da última actualização |
| `indicador_produto_id` | string | ID do indicador do produto |
| `id_tipo_produto` | integer | ID do tipo de produto |
| `registo_produto` | string | Descrição do registo do produto (ex: `Pagamento/Carregamento`) |
| `id_produto` | integer | ID do produto |
| `cliente_tipo_produto` | integer | ID do tipo de produto do cliente |
| `criado_quando` | string\<date-time\> | Data/hora de criação do produto associado |
| `produto` | string | Nome do produto (ex: `Paga Só Pagamentos`, `PAYKWANZA`, `API SERVIÇOS - INTELIZE`, `Portal Intelize Pagamentos`) |
| `codigo_do_produto` | integer | Código do produto |

### Pagamento

*(inclui todos os campos de Referência listados acima, fundidos com os seguintes campos específicos da transacção)*

| Campo | Tipo | Descrição |
| --- | --- | --- |
| `id_pagamento` | integer | ID interno do pagamento |
| `montante_da_operacao` | string | Valor pago na operação |
| `montante_total_transicoes` | string | Total acumulado de transacções (formato numérico com zeros à esquerda) |
| `data_do_processamento` | string\<date\> | Data de processamento do pagamento |
| `id_ultimo_ficheiro` | string | ID do último ficheiro de compensação processado |
| `tarifa_aplicada_a_operacao` | string | Tarifa aplicada à operação |
| `total_transicao` | string | Total da transacção |
| `referencia_do_servico` | string | Referência do serviço (equivalente a `num_referencia`) |
| `numero_entidade` | string | Número de entidade |
| `data_hora_transacao_cliente` | string\<date-time\> | Data/hora da transacção do lado do cliente |
| `codigo_produto` | string | Código do produto |
| `operacao_Pagamento` | string | Código do tipo de operação de pagamento |
| `localidade` | string | Localidade/canal onde o pagamento foi efectuado (ex: `Internet`, `LUANDA`) |
| `modo_de_Envio_Comunicacao` | string | Modo de envio da comunicação |
| `codigo_de_Resposta_da_Empresa` | string | Código de resposta da empresa |
| `n_de_Identificacao_da_Resposta` | string | Número de identificação da resposta |
| `PRT` | string | Estado do protocolo (ex: `ACEITE`) |
| `MFT` | string | Estado da mensagem financeira (ex: `CONCLUIDO`) |
| `aplicaccao_PDD` | string | Código de aplicação PDD |
| `Identificacao_Log_EGR` | string | Identificação do log EGR |
| `numero_Log_EGR` | string | Número do log EGR |
| `codigo_parametro` | string | Código de parâmetro |
| `data_movimento` | string | Data do movimento (código) |
| `hora_do_movimento` | string | Hora do movimento (código) |
| `tipo_de_Terminal` | string | Tipo de terminal (ex: `M` = Mobile/Internet, `A` = ATM) |
| `numero_Periodo_Contabilistico` | string | Número do período contabilístico |
| `identificacao_Transacao_Local` | string | Identificação da transacção local |
| `identificacao_do_Terminal` | string | Identificação do terminal físico |
| `RFU` | string | Campo reservado para uso futuro (Reserved for Future Use) |
| `codigo_de_Moeda` | string | Código ISO numérico da moeda (`024` = AOA/Kwanza) |

---

## Inconsistências detectadas na coleção original

Durante a extracção desta documentação a partir do ficheiro `.json` exportado, foram identificadas as seguintes inconsistências na coleção Postman de origem — não são falhas da API em si, mas sim problemas na coleção de testes/exemplos que a Intelize disponibilizou. Recomenda-se validar directamente com a Intelize (ou por testes reais) antes de assumir os detalhes reconstruídos:

1. **`Referências de pagamentos > Pelo ID`** e **`Referências de pagamentos > Intervalo de dias`**: o campo `url.raw` do pedido principal estava **vazio** na coleção exportada. As URLs documentadas acima (`/auth/referencias/{id_referencia}` e `/auth/referencias/dia-inicio/{data_inicio}/dia-final/{data_final}`) foram reconstruídas a partir do `originalRequest` gravado dentro do exemplo de resposta salvo, que ainda continha a URL correcta usada na altura da captura.

2. **`Pagamentos > Pelo ID`**: o pedido principal gravado na coleção apontava (provavelmente por engano de duplicação no Postman) para a mesma URL do endpoint `Pagamentos > Por referência` (`/auth/pagamentos/referencia/747190407`). O exemplo de resposta gravado, no entanto, foi capturado a partir de `/auth/pagamentos/22` (um ID numérico), o que é consistente com o nome do endpoint ("Pelo ID"). A documentação acima segue o exemplo de resposta (URL por ID), não o pedido principal mal configurado.

3. **`Referências de pagamentos > Deletar referências`**: o pedido `DELETE` tinha um corpo de exemplo (`{"numero_de_linhas": 2}`) idêntico ao do `PATCH` logo acima — isto é atípico para um método `DELETE` e é provavelmente um resíduo de cópia/duplicação do pedido `PATCH`. Não há evidência, nos exemplos de resposta, de que este corpo seja necessário ou processado pela API.

4. **Token de autenticação exposto**: a coleção original tinha um Bearer Token JWT válido gravado como valor por omissão no nível de autenticação da colecção. Foi **removido/substituído por `<token>`** nesta documentação — ver aviso na secção [Autenticação](#autenticação).

Nenhuma destas inconsistências impede o uso da documentação, mas convém confirmá-las com a equipa da Intelize (ou com testes reais na API) antes de construir integrações críticas em cima delas — especialmente os pontos 2 e 3.
