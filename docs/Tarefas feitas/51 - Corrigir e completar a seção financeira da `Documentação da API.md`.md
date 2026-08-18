# Tarefa para o Codex — Corrigir e completar a seção financeira da `Documentação da API.md`

> **Este documento já contém todas as decisões e fatos necessários.** Você (Codex) não precisa
> investigar o "porquê" de nenhuma regra nem decidir formato — apenas **escrever o texto final**
> seguindo exatamente as instruções abaixo, no arquivo `Documentação da API.md` (raiz do repositório).
> Todos os fatos de negócio abaixo foram extraídos diretamente do código-fonte Go (handlers,
> aggregates, `internal/finance/*`, projections, migrations) por outra instância de IA (Claude), que
> revisou cada arquivo relevante antes de escrever este brief. Não é necessário reler o código para
> validar os fatos de negócio descritos aqui — eles já estão corretos. Você só precisa consultar o
> código-fonte se precisar copiar literalmente um nome de campo/constante que não esteja 100% claro
> abaixo.

## 0. Ambiente e o que NÃO é necessário

- Esta tarefa é **exclusivamente de documentação**: um único arquivo Markdown (`Documentação da
  API.md`) é editado. **Nenhum arquivo `.go` deve ser alterado.**
- **Não é necessário** banco de dados, Docker, `psql`, `go build`, `go test` ou subir o servidor.
  Sabemos que seu ambiente bloqueia `apt` e não tem Docker/`psql` — isso não importa aqui, porque
  nada nesta tarefa depende disso.
- Autovalidação permitida/esperada (sem infraestrutura): conferir que blocos de código JSON estão
  bem formados, que não sobraram números de seção duplicados/quebrados, que os links do índice
  continuam batendo com os títulos, e que a numeração ficou sequencial. Isso pode ser feito só lendo
  o Markdown.
- Ao final, a revisão de exatidão factual (comparar o nível de detalhe do texto com o comportamento
  real do backend) será feita por mim (Claude/orquestrador), não por você. Você não precisa rodar
  nada para "provar" que está certo — apenas seguir este brief com fidelidade.

## 1. Escopo exato — só toque nisto

Arquivo: `Documentação da API.md` (raiz do repo).

Seções que **serão editadas**:

- Índice (topo do arquivo, itens da seção 9)
- `### 2.5 SolicitacaoMatricula`
- `## 9. Solicitação de Matrícula` (adição de 9.9–9.12 e reescrita de 9.5)
- `## 19. Financeiro / AppyPay` (renumeração parcial + grandes adições)
- A seção solta no fim do arquivo, `## Cobrança de matrícula por solicitação` — **será removida**
  depois que todo o seu conteúdo tiver sido redistribuído (de forma mais completa) para dentro das
  seções 9 e 19.

**Não toque em mais nada.** Não "aproveite" para corrigir outras seções do documento mesmo que
perceba outros problemas nelas (ex.: a interface `SolicitacaoMatriculaDTO` da seção 2.5 tem um outro
campo que falta, `telefone_encarregado` — **inclua esse campo específico** porque ele é necessário
para os exemplos desta tarefa ficarem corretos, mas não vá procurar outros problemas fora do escopo
listado acima).

## 2. Convenções de formatação — siga à risca

O documento tem **dois estilos diferentes** de bloco de endpoint, dependendo da seção. Reproduza
exatamente o estilo já usado na seção onde você está escrevendo.

### 2.1 Estilo da Seção 9 (use para as novas 9.9–9.12)

Cabeçalho nível 3, com o método e a rota entre crases:

```
### 9.9 `GET /solicitacao-matricula/busca`

Texto curto de uma linha descrevendo o objetivo.

**Proteção:** ...

**Query params:** (ou **Request fields:** / **Request body:**, conforme o caso)

| Campo | Tipo | Obrigatório | Observações |
| --- | --- | --- | --- |

**Exemplo de request:**

```http
GET /solicitacao-matricula/busca?... HTTP/1.1
```

**Response 200:**

```json
{ ... }
```

**Regras de negócio:**

- ...

**Erros comuns:** `400` ..., `403` ..., `404` ...
```

Use exatamente os mesmos nomes de subtítulo (`**Proteção:**`, `**Regras de negócio:**`,
`**Erros comuns:**` etc.) já usados em 9.1–9.8. Não invente novos rótulos.

### 2.2 Estilo da Seção 19 (use para as novas 19.9–19.22)

Cabeçalho nível 4, **sem crase** no método/rota:

```
#### 19.14 POST /financeiro/mensalidades/configuracoes

**Escopo da rota:** ...

**Proteção:** ...

**Request JSON:**

```json
{ ... }
```

**Response 201:**

```json
{ ... }
```

**Regras de negócio:**

- ...
```

Use exatamente os mesmos rótulos já usados em 19.1–19.8 e 19.10–19.14 (`**Escopo da rota:**`,
`**Proteção:**`, `**Request JSON:**` ou `**Campos do request:**` com tabela quando há muitos campos,
`**Response NNN:**`, `**Regras de negócio:**`).

### 2.3 Tom e idioma

Português europeu/angolano formal, igual ao resto do documento (é o mesmo tom já usado em todas as
seções — "cobrança", "propina/mensalidade", "matrícula", etc.). Explicações **didáticas**: quando uma
regra depende de um conceito (ex.: "posição no ano letivo", "versão vigente na data de referência"),
explique em uma frase o que isso significa antes de aplicá-lo, do mesmo jeito que a seção 19.9 atual
já faz ao explicar a resolução de preço por mês.

---

## 3. PARTE A — Renumeração da Seção 19 e correção de referências cruzadas

A seção 19 hoje vai de 19.1 a 19.14, mas **19.9 é um bloco de prosa genérico** (mensalidades) que
mistura 6 rotas sem o formato padrão, e as rotas de **taxa de matrícula** (`/financeiro/matriculas/
configuracoes`) não estão documentadas dentro da seção 19 — estão resumidas, de forma incompleta, na
seção solta no final do arquivo.

Plano de renumeração (faça exatamente isto, nesta ordem):

1. **Mantenha inalterados**: 19.1, 19.2, 19.3, 19.4, 19.5, 19.6, 19.7, 19.8 (título, conteúdo e
   qualquer referência a eles em outros pontos do texto continuam iguais).
2. **Remova inteiramente o bloco atual `#### 19.9 Mensalidades/propinas e pagamento pelo estudante`**
   (da linha do cabeçalho até imediatamente antes de `#### 19.10 POST /financeiro/appypay/cobrancas/:id/cancelar`).
   Todo o conteúdo de negócio que está nesse bloco de prosa será reescrito de forma completa e
   correta nas novas seções 19.14–19.20 (Parte B abaixo) — não é preciso preservar o texto atual
   literalmente, apenas garantir que nenhuma regra de negócio ali descrita se perca.
3. **Renumere** os blocos seguintes, só no número do cabeçalho (o conteúdo de cada um permanece
   igual, exceto pelas correções pontuais indicadas abaixo):
   - `#### 19.10 POST /financeiro/appypay/cobrancas/:id/cancelar` → `#### 19.9 POST /financeiro/appypay/cobrancas/:id/cancelar`
   - `#### 19.11 POST /webhooks/appypay/gpo` → `#### 19.10 POST /webhooks/appypay/gpo`
   - `#### 19.12 POST /webhooks/appypay/ref` → `#### 19.11 POST /webhooks/appypay/ref`
   - `#### 19.13 GET /financeiro/appypay/credenciais/:id/webhook-secret` → `#### 19.12 GET /financeiro/appypay/credenciais/:id/webhook-secret`
   - `#### 19.14 POST /financeiro/appypay/credenciais/:id/webhook-secret/rotacionar` → `#### 19.13 POST /financeiro/appypay/credenciais/:id/webhook-secret/rotacionar`
4. **Insira as novas seções 19.14 a 19.22** (conteúdo completo nas Partes B e C abaixo) logo depois
   do novo 19.13 e **antes** da tabela final `**Erros comuns das rotas autenticadas:**` (essa tabela
   continua sendo o fechamento da seção 19, agora depois de 19.22).

### 3.1 Correções de referências cruzadas (texto que cita "seção 19.X")

O documento atual já tem **três erros de referência cruzada preexistentes** (não introduzidos por
você — já estavam errados antes desta tarefa). Corrija-os como parte da renumeração, usando a nova
numeração:

| Localização (bloco onde aparece) | Texto atual (errado) | Texto corrigido |
| --- | --- | --- |
| Bullet "Segredos AppyPay..." na introdução da seção 19 | `...apenas na criação da credencial (seção 19.1) e nas rotas dedicadas de consulta/rotação (seções 19.10 e 19.11)...` | `...apenas na criação da credencial (seção 19.1) e nas rotas dedicadas de consulta/rotação (seções 19.12 e 19.13)...` |
| Dentro do bloco 19.2 (`PUT .../credenciais/:id`), frase "Não altera o segredo de webhook..." | `...use \`POST .../webhook-secret/rotacionar\` (seção 19.11).` | `...use \`POST .../webhook-secret/rotacionar\` (seção 19.13).` |
| Dentro do bloco 19.2, regra de negócio sobre rotação | `...ele só muda por rotação explícita (\`POST .../webhook-secret/rotacionar\`, seção 19.11).` | `...ele só muda por rotação explícita (\`POST .../webhook-secret/rotacionar\`, seção 19.13).` |
| Dentro do bloco 19.3 (`GET .../credenciais`), regra de negócio final | `...O segredo de webhook tem rotação própria (\`POST .../credenciais/:id/webhook-secret/rotacionar\`, seção 19.11) e nunca aparece mascarado aqui — só em texto pleno pelas rotas dedicadas (seções 19.1, 19.10 e 19.11).` | `...O segredo de webhook tem rotação própria (\`POST .../credenciais/:id/webhook-secret/rotacionar\`, seção 19.13) e nunca aparece mascarado aqui — só em texto pleno pelas rotas dedicadas (seções 19.1, 19.12 e 19.13).` |
| Dentro do (agora) 19.13 (`POST .../webhook-secret/rotacionar`), campo **Proteção** | `**Proteção:** igual à seção 19.10.` | `**Proteção:** igual à seção 19.12.` |
| Dentro do (agora) 19.13, campo **Response 200** | `**Response 200:** igual à seção 19.10, com o novo valor de \`webhook_secret\`.` | `**Response 200:** igual à seção 19.12, com o novo valor de \`webhook_secret\`.` |

Todas as demais referências (`use 19.6`, `use 19.7`, `use 19.8`, `Mesmo filtro de 19.7`, `mesma
estrutura de 19.7`, `Diferente de 19.7`, `mesmo campo documentado em 19.5`) **não mudam** — continuam
corretas porque 19.5, 19.6, 19.7 e 19.8 não foram renumeradas.

### 3.2 Atualize a tabela de rotas no topo da seção 19

A tabela `| Método | Rota | Escopo resumido |` logo após os bullets de "Regras gerais do escopo
financeiro" já lista as rotas de credenciais, cobranças, QR code, webhooks e mensalidades — mas **não
lista as 3 rotas de configuração de taxa de matrícula**. Adicione estas três linhas à tabela, no
mesmo estilo das demais (pode posicioná-las logo depois das linhas de `/financeiro/mensalidades/*`):

```
| `POST` | `/financeiro/matriculas/configuracoes` | Versiona valor e métodos de pagamento da taxa de matrícula por nível/ano/curso de uma academia. |
| `PUT` | `/financeiro/matriculas/configuracoes` | Idêntico ao `POST` acima — mesma rota, mesmo efeito (nova versão da configuração). |
| `GET` | `/financeiro/matriculas/configuracoes` | Lista as configurações vigentes de taxa de matrícula de uma academia. |
```

### 3.3 Bullet adicional na introdução ("Regras gerais do escopo financeiro")

Adicione **um novo bullet** à lista de "Regras gerais do escopo financeiro" (no início da seção 19,
antes da tabela de rotas), explicando que a matrícula (taxa de admissão) tem seu próprio fluxo
público de cobrança fora de `/financeiro/*`, com referência cruzada para a nova seção 19.21 e para a
seção 9. Redija de forma natural, no mesmo tom dos bullets vizinhos; o conteúdo factual a transmitir
é:

- A taxa de matrícula é configurada pela academia/admin FPP em `/financeiro/matriculas/configuracoes`
  (seção 19.21), mas **a cobrança em si nunca é criada por uma rota de `/financeiro/*`**: ela nasce
  automaticamente quando o próprio candidato aprovado paga através das rotas públicas descritas na
  seção 9 (9.10 e 9.11).

---

## 4. PARTE B — Novas seções 19.14 a 19.20 (Mensalidades/propinas)

Estas sete rotas hoje só têm um parágrafo de prosa (o antigo 19.9, que você já removeu na Parte A).
Escreva cada uma no formato completo da seção 19 (ver 2.2), usando os fatos abaixo. Todas ficam sob
`/financeiro/mensalidades/*`.

### Regra transversal a mencionar antes de 19.14 (frase de contexto, como já existe hoje)

Mantenha, como parágrafo introdutório antes do primeiro endpoint (19.14), uma versão didática do
texto que já existe hoje no início do antigo 19.9 — explicando os conceitos que todas as rotas de
mensalidade compartilham:

- A obrigação mensal usa a chave estável `(codigo_estudante, codigo_academia, ano_letivo, mes)`.
- Só academias **privadas** (`type = "private"` no projection de academias) podem configurar e gerar
  mensalidades; uma academia pública nunca tem propina.
- Cada mês é resolvido dinamicamente (nunca por um valor gravado por estudante): o preço aplicado é o
  que estava **vigente no primeiro dia daquele mês** (não o preço atual), e a turma/curso considerados
  são os **históricos** do estudante naquele ano letivo (via `historico_estudantes_ano_letivo` da
  turma) — por isso um mês antigo nunca muda de valor mesmo que o estudante troque de turma/curso
  depois.
- `mes_fim_cobranca` só aceita `6` ou `7` (o período letivo é fixo; propina nunca é cobrada além
  disso).
- O mês de início natural do ano letivo é setembro para fundamental/médio e outubro para superior;
  `POST /financeiro/mensalidades/inicio-cobranca` é a única forma de encurtar esse início, e vale
  estritamente para o par `(academia, ano_letivo)` informado — não altera períodos letivos.
- Todo valor é `float64`, arredondado por *half away from zero* a duas casas (mesma regra do resto
  do módulo financeiro — reforce que é a mesma regra da seção 19, não uma regra nova).

### 19.14 `POST` e `PUT /financeiro/mensalidades/configuracoes`

- **Handler:** `ConfigurarMensalidade` (ambos os verbos HTTP apontam para o mesmo handler).
- **Proteção:** academia dona (código forçado pelo token) ou admin FPP (precisa da permissão `fpp`;
  para admin, `codigo_academia` é obrigatório no corpo — sem ele, `403`).
- **Fato importante a explicar (didático):** aqui `PUT` **não é uma atualização in-place por id**.
  A configuração é *versionada por evento*: cada chamada válida — seja `POST` ou `PUT` — sempre grava
  uma **nova versão** com `vigente_em` = o instante do servidor no momento do registro. Não existe
  "editar" uma versão existente; por isso `PUT` não leva `:id` na URL e tem exatamente o mesmo efeito
  do `POST`. Deixe isso bem explícito para o leitor não estranhar um `PUT` sem identificador de
  recurso.
- **Request JSON** (campos e regras):

  | Campo | Tipo | Obrigatório | Regras |
  | --- | --- | --- | --- |
  | `codigo_academia` | string | Sim (forçado pelo token quando quem chama é academia) | Deve existir e ser do tipo `private`. |
  | `nivel` | string | Sim | `fundamental`, `medio` ou `superior`. |
  | `ano_academico` | string | Sim | Deve ser um dos anos oferecidos pela academia/curso (ex.: `1_ano_medio`). |
  | `curso_id` | UUID | Obrigatório para `medio`/`superior`; proibido para `fundamental` | Curso deve pertencer à academia, ter o `type` igual a `nivel` e oferecer o `ano_academico` informado. |
  | `valor` | número (`float64`) | Sim | Maior que zero, no máximo duas casas decimais. |
  | `mes_fim_cobranca` | inteiro | Sim | Só aceita `6` ou `7`. |
  | `metodos_pagamento` | array de string | Não (pode ser vazio) | Cada item deve ser `GPO`, `REF` ou `GPO_QR`, sem duplicados. Uma lista vazia **desativa** o pagamento de propina para essa combinação nível/ano/curso (nenhum método habilitado). Quando a lista não é vazia, a academia precisa ter credencial AppyPay configurada (seção 19.1) — sem credencial, erro de validação. |

  Exemplo (médio, com curso):
  ```json
  {
    "codigo_academia": "ACA001",
    "nivel": "medio",
    "ano_academico": "1_ano_medio",
    "curso_id": "550e8400-e29b-41d4-a716-446655440000",
    "valor": 25000.00,
    "mes_fim_cobranca": 7,
    "metodos_pagamento": ["REF", "GPO_QR"]
  }
  ```

- **Response 201** (`MensalidadeConfiguracaoView`, mesmo em `PUT`):
  ```json
  {
    "codigo_academia": "ACA001",
    "nivel": "medio",
    "ano_academico": "1_ano_medio",
    "curso_id": "550e8400-e29b-41d4-a716-446655440000",
    "valor": 25000.00,
    "mes_fim_cobranca": 7,
    "metodos_pagamento": ["REF", "GPO_QR"],
    "vigente_em": "2026-08-08T12:00:00Z"
  }
  ```
- **Regras de negócio** (a incluir em bullets, com suas próprias palavras, cobrindo todos os fatos
  acima): tipo de academia deve ser `private`; `curso_id` proibido em fundamental e obrigatório em
  médio/superior; validação de que o ano/curso é realmente oferecido pela academia; `mes_fim_cobranca`
  limitado a 6/7; cada chamada gera sempre uma nova versão (nunca sobrescreve, nunca há checagem de
  "já existe uma igual" — reenviar o mesmo payload cria uma nova versão idêntica, desnecessária mas
  válida); histórico de versões é preservado para sempre, para não afetar a resolução de preço de
  meses passados.
- **Erros comuns:** `400` payload/regra de negócio inválida (inclusive "academia não encontrada",
  que aqui sai como `404` — ver regra transversal na Parte da seção 7 deste brief), `403` sem
  permissão.

### 19.15 `GET /financeiro/mensalidades/configuracoes`

- **Proteção:** igual à 19.14 (academia dona ou admin FPP com `codigo_academia` obrigatório).
- **Query params:** `codigo_academia` (obrigatório para admin; forçado pelo token para academia).
- **Response 200:**
  ```json
  {
    "codigo_academia": "ACA001",
    "configuracoes": [
      {
        "codigo_academia": "ACA001",
        "nivel": "medio",
        "ano_academico": "1_ano_medio",
        "curso_id": "550e8400-e29b-41d4-a716-446655440000",
        "valor": 25000.00,
        "mes_fim_cobranca": 7,
        "metodos_pagamento": ["REF", "GPO_QR"],
        "vigente_em": "2026-08-08T12:00:00Z"
      }
    ]
  }
  ```
- **Regras de negócio:** devolve **apenas a versão mais recente** (`vigente_em` mais alto) de cada
  combinação distinta `(nivel, ano_academico, curso_id)` — não é um histórico de versões, é a "foto"
  do que está vigente agora para cada trilha; para resolver o preço de um mês específico no passado,
  use a consulta de mensalidades do estudante (19.17), que aplica a versão correta por data de
  referência.

### 19.16 `POST /financeiro/mensalidades/inicio-cobranca`

- **Escopo da rota:** define uma exceção pontual de início de cobrança para uma academia que entrou
  no ano letivo corrente fora do mês natural (setembro/outubro) — por exemplo, uma academia nova que
  só começou a operar em janeiro.
- **Proteção:** academia dona ou admin FPP.
- **Request JSON:**
  ```json
  { "codigo_academia": "ACA001", "ano_letivo": "2026_2027", "mes_inicio": 1 }
  ```
- **Response 201:** corpo vazio (a rota não devolve JSON — apenas o status).
- **Regras de negócio:**
  - `ano_letivo` deve estar no formato `YYYY_YYYY` com o segundo ano igual ao primeiro + 1.
  - Academia deve ser `private` (mesma regra de 19.14).
  - `mes_inicio` é validado por **posição dentro do ano letivo** (não por número de mês do
    calendário): não pode ser posterior ao menor `mes_fim_cobranca` já configurado para essa
    academia, comparando pela mesma noção de "posição no ano letivo" usada para resolver os meses
    devidos (explique brevemente essa noção: os meses são contados a partir do mês natural de início
    — setembro para fundamental/médio, outubro para superior — então "mês 1" é sempre o de início e
    "mês 11" é julho do ano seguinte, por exemplo).
  - Vale estritamente para o par `(academia, ano_letivo)` informado; não afeta outros anos letivos
    nem os períodos letivos imutáveis do sistema.
- **Erros comuns:** `400` para dados inválidos ou `mes_inicio` posterior ao fim configurado, `404`
  para academia inexistente, `403` sem permissão.

### 19.17 `GET /financeiro/mensalidades/estudante/:codigo`

- **Escopo da rota:** calcula sob consulta (não é uma tabela gravada por mês) todos os meses devidos,
  pagos ou anulados de um estudante, em todas as academias privadas onde ele tem vínculo histórico ou
  atual.
- **Proteção:** o próprio estudante (só o seu código), a academia com vínculo histórico/atual com
  aquele estudante (restrita às próprias mensalidades), ou admin FPP (permissão `fpp`, qualquer
  estudante).
- **Path params:** `codigo` — código do estudante.
- **Response 200:**
  ```json
  {
    "codigo_estudante": "EST0001",
    "mensalidades": [
      {
        "codigo_estudante": "EST0001",
        "codigo_academia": "ACA001",
        "ano_letivo": "2025_2026",
        "mes": 10,
        "data_referencia": "2025-10-01T00:00:00Z",
        "nivel": "medio",
        "ano_academico": "1_ano_medio",
        "curso_id": "550e8400-e29b-41d4-a716-446655440000",
        "valor": 25000.00,
        "mes_fim_cobranca": 7,
        "estado": "pendente",
        "eventos_auditoria": []
      }
    ],
    "metodos_pagamento_por_academia": {
      "ACA001": ["REF", "GPO_QR"]
    }
  }
  ```
- **Regras de negócio:**
  - `estado` é sempre um de `pendente`, `pago` ou `anulado`; a precedência entre eventos é: um
    pagamento real (`paga`) sempre vence, mesmo que uma anulação tenha sido registrada depois; uma
    reativação só tem efeito se o mês estava `anulado` (reativar um mês que já está `pendente` não
    faz nada de diferente, e reativar um mês `pago` é bloqueado na própria rota de reativação — ver
    19.20).
  - `eventos_auditoria` lista os `event_id` (UUID) de todos os eventos de anulação/reativação/
    pagamento já aplicados àquele mês, na ordem em que ocorreram — útil para rastrear o histórico
    completo de um mês específico.
  - `metodos_pagamento_por_academia` reflete a configuração vigente mais recente de cada academia
    onde o estudante tem mensalidade listada; **uma lista vazia significa que a propina está
    desativada** para pagamento naquela academia (mesmo que existam meses pendentes listados).
  - A academia autenticada só recebe as obrigações da sua própria chave de academia (não vê
    mensalidades que o estudante tem em outra academia, mesmo com vínculo histórico duplo); o
    estudante e o admin FPP recebem os grupos de todas as academias.
- **Erros comuns:** `404` estudante inexistente, `403` estudante tentando ver outro código, academia
  sem vínculo com o estudante, ou admin sem permissão `fpp`.

### 19.18 `POST /financeiro/mensalidades/pagamento`

- **Escopo da rota:** único caminho pelo qual um estudante paga propina — cria automaticamente **uma
  única cobrança** cobrindo os meses escolhidos, na academia indicada. Exclusivo do próprio
  estudante autenticado (nem academia nem admin podem chamar esta rota em nome dele).
- **Proteção:** autenticado como `estudante`. Qualquer outro tipo de usuário recebe `403`.
- **Request JSON:**
  ```json
  {
    "codigo_academia": "ACA001",
    "meses": [
      { "ano_letivo": "2025_2026", "mes": 10 },
      { "ano_letivo": "2025_2026", "mes": 1 }
    ],
    "metodo_pagamento": "REF"
  }
  ```
  `telefone` é aceite e obrigatório apenas quando `metodo_pagamento` é `GPO` (mesma regra de
  `paymentInfo.phoneNumber` da seção 19.4).
- **Response 201** (`MensalidadePagamentoView`):
  ```json
  {
    "cobranca": {
      "id": "4d2bbf53-c8c0-4c9a-a3f4-5a0f0cf988d1",
      "provider_charge_id": "APPYPAY-987654",
      "merchant_transaction_id": "PROP2608LDA0001",
      "status": "pendente",
      "response": { "status": "Accepted" }
    },
    "meses": [
      { "ano_letivo": "2025_2026", "mes": 10 },
      { "ano_letivo": "2025_2026", "mes": 1 }
    ]
  }
  ```
  Quando `metodo_pagamento` é `GPO_QR`, o objeto `cobranca` inclui também `"qrCodeArr"` em base64
  (mesmo campo/formato da seção 19.5); para os demais métodos o campo fica ausente.
- **Regras de negócio:**
  - `metodo_pagamento` precisa estar na lista de métodos habilitados **daquela academia** (a mesma
    lista devolvida em `metodos_pagamento_por_academia` por 19.17); caso contrário, erro de
    validação.
  - A seleção de meses é validada **inteiramente antes** de chamar a AppyPay: cada mês precisa estar
    atualmente `pendente` (não pago, não anulado, não duplicado na mesma seleção) e **sem** cobrança
    aberta já existente cobrindo aquele mês.
  - **A seleção deve obrigatoriamente incluir o mês pendente mais antigo daquela academia** — o
    estudante não pode "pular" o mês mais antigo em aberto; os demais meses escolhidos podem ser
    quaisquer outros meses pendentes da mesma academia (não precisam ser consecutivos).
  - O valor da cobrança é a soma dos preços **históricos** de cada mês selecionado (cada mês usa o
    preço que estava vigente na sua própria data de referência — ver introdução da Parte B),
    arredondada pela regra financeira única do módulo (seção 19, *half away from zero*, duas casas).
  - Uma única cobrança é criada cobrindo todos os meses selecionados — nunca uma cobrança por mês.
  - Confirmação: quando a AppyPay reporta sucesso (por consulta síncrona — seção 19.6 — ou por
    webhook — 19.10/19.11), o evento `MensalidadesCobrancaConfirmada` grava o pagamento de **todos os
    meses da cobrança em uma única transação** na projeção; se o mesmo sucesso for processado mais de
    uma vez (reentrega de webhook, ou consulta repetida), a operação é idempotente e não duplica o
    pagamento.
- **Erros comuns:** `400` para qualquer violação das regras acima (não é apenas conflito de dados —
  ver a regra transversal de mapeamento de erros na seção 7 deste brief: mesmo mensagens que soam como
  "já existe cobrança em aberto" respondem `400`, não `409`, neste endpoint específico), `403` para
  quem não é estudante.

### 19.19 `POST /financeiro/mensalidades/obrigacoes/anular`

- **Escopo da rota:** a academia isenta o estudante do pagamento de um ou mais meses específicos
  (ex.: bolsa social), registrando um evento de anulação por mês — nunca apaga histórico, apenas
  acrescenta um novo evento imutável.
- **Proteção:** **exclusiva da academia dona** do vínculo (código forçado pelo token). **Admin FPP
  recebe `403`** mesmo tendo acesso de leitura ao resto do módulo financeiro — esta é uma decisão de
  negócio da própria academia, não do FPP.
- **Request JSON:**
  ```json
  {
    "codigo_estudante": "EST0001",
    "codigo_academia": "ACA001",
    "ano_letivo": "2026_2027",
    "meses": [1, 2],
    "motivo": "bolsa social"
  }
  ```
  `motivo` é opcional mas recomendado para auditoria.
- **Response 201:** corpo vazio.
- **Regras de negócio:**
  - A academia autenticada precisa ter vínculo (atual ou histórico, via turma) com o estudante —
    caso contrário, `403`.
  - `meses` deve ser uma lista de inteiros distintos entre 1 e 12; cada mês precisa corresponder a um
    mês efetivamente devido (dentro do período de mensalidade configurado daquele ano letivo) —
    senão, erro de validação.
  - **Não é possível anular um mês já pago** (erro de validação).
  - Cada mês gera seu próprio evento `ObrigacaoMensalidadeAnulada`; eventos anteriores (inclusive
    pagamentos ou anulações antigas) nunca são apagados ou substituídos.
  - **Efeito colateral sobre cobranças abertas:** se algum dos meses anulados fazia parte de uma
    cobrança ainda aberta (não paga, não cancelada, não falhada) — inclusive uma cobrança que também
    cobria outros meses ainda não anulados — essa cobrança inteira é cancelada localmente (mesmo
    mecanismo da seção 19.9/cancelamento manual). Os demais meses que estavam nessa cobrança **não
    são marcados como anulados** (seu estado individual continua sendo o que já era, tipicamente
    `pendente`) — eles apenas deixam de ter uma cobrança aberta bloqueando-os, ficando livres para
    receber uma nova cobrança em uma próxima tentativa de pagamento.
  - Esse cancelamento de cobrança é best-effort: se falhar (ex.: a cobrança acabou de ser paga na
    AppyPay entre a validação e o cancelamento), a anulação do(s) mês(es) já foi gravada e permanece
    válida; a cobrança em conflito segue o mesmo tratamento de reconciliação manual FPP descrito na
    seção 19.9 original (evento `CobrancaAppyPayConflitoPosCancelamento`).
- **Erros comuns:** `400` para mês já pago, mês fora do período configurado, ou dados inválidos,
  `403` para academia sem vínculo com o estudante ou para admin FPP.

### 19.20 `POST /financeiro/mensalidades/obrigacoes/reativar`

- **Escopo da rota:** reverte uma anulação, voltando o(s) mês(es) para `pendente`.
- **Proteção:** idêntica à 19.19 (exclusiva da academia dona; admin FPP recebe `403`).
- **Request JSON:** mesmo formato de 19.19.
- **Response 201:** corpo vazio.
- **Regras de negócio:**
  - **Só é possível reativar um mês que está atualmente `anulado`** — tentar reativar um mês
    `pendente` ou `pago` é erro de validação.
  - Cada mês gera seu próprio evento `ObrigacaoMensalidadeReativada`; não há efeito sobre cobranças
    (reativar não cria nem cancela cobrança alguma).
- **Erros comuns:** `400` para mês que não está anulado, `403` para academia sem vínculo ou admin FPP.

---

## 5. PARTE C — Novas seções 19.21 e 19.22 (Taxa de matrícula)

Insira logo depois de 19.20, com um parágrafo introdutório curto explicando que esta é a
**configuração** da taxa de matrícula (o pagamento em si, feito pelo candidato, é público e mora na
seção 9 — 9.10/9.11; aqui é só o painel de configuração da academia/FPP).

### 19.21 `POST` e `PUT /financeiro/matriculas/configuracoes`

- **Handler:** `ConfigurarMatricula` (ambos os verbos apontam para o mesmo handler; mesmo raciocínio
  de "sempre uma nova versão, nunca update in-place" explicado em 19.14 — repita brevemente essa
  explicação aqui também, não pressuponha que o leitor leu 19.14).
- **Proteção:** academia dona (código forçado pelo token) ou admin FPP (`codigo_academia`
  obrigatório).
- **Diferença importante em relação à mensalidade (destaque isso explicitamente):** ao contrário da
  mensalidade, **a taxa de matrícula pode ser configurada tanto por academias públicas quanto
  privadas** — não há restrição de `type = "private"` aqui.
- **Request JSON e regras dos campos:**

  | Campo | Tipo | Obrigatório | Regras |
  | --- | --- | --- | --- |
  | `codigo_academia` | string | Sim (forçado pelo token para academia) | Academia deve existir. |
  | `nivel` | string | Sim | `fundamental`, `medio` ou `superior`. |
  | `ano_academico` | string | Sim | Deve ser oferecido pela academia/curso. |
  | `curso_id` | UUID | Obrigatório para `medio`/`superior`; proibido para `fundamental` | Mesmas regras de vínculo curso↔academia↔ano de 19.14. |
  | `valor` | número (`float64`) | Sim | Maior que zero, no máximo duas casas decimais. |
  | `metodos_pagamento` | array de string | **Sim, com pelo menos um item** (diferente da mensalidade, aqui não pode ser vazio) | `GPO`, `REF` ou `GPO_QR`, sem duplicados. Exige credencial AppyPay já configurada para a academia. |

  Exemplo:
  ```json
  {
    "codigo_academia": "ACA001",
    "nivel": "medio",
    "ano_academico": "1_ano_medio",
    "curso_id": "550e8400-e29b-41d4-a716-446655440000",
    "valor": 15000.00,
    "metodos_pagamento": ["REF", "GPO"]
  }
  ```
- **Response 201** (`MatriculaConfiguracaoView`, mesmo em `PUT`):
  ```json
  {
    "codigo_academia": "ACA001",
    "nivel": "medio",
    "ano_academico": "1_ano_medio",
    "curso_id": "550e8400-e29b-41d4-a716-446655440000",
    "valor": 15000.00,
    "metodos_pagamento": ["REF", "GPO"],
    "vigente_em": "2026-08-08T12:00:00Z"
  }
  ```
- **Regras de negócio adicionais, essenciais (explique com destaque — isto é o coração do fluxo de
  matrícula paga):**
  - **Sem nenhuma configuração cadastrada para a combinação nível/ano/curso de uma solicitação, a
    matrícula continua gratuita**: a aprovação da solicitação (seção 9.5) cria o vínculo do estudante
    imediatamente, exatamente como sempre funcionou.
  - **Com uma configuração vigente**, a aprovação de uma solicitação para aquela combinação
    nível/ano/curso passa a exigir pagamento antes de o estudante ser efetivamente criado — ver
    reescrita completa da seção 9.5 (Parte E deste brief) e o novo fluxo público 9.10/9.11.
  - O valor e os métodos aplicados a uma solicitação já aprovada ficam **congelados** no momento da
    aprovação (gravados na própria solicitação); alterar a configuração depois não muda o valor que o
    candidato já tem para pagar.
- **Erros comuns:** `400` payload/regra inválida, `404` academia/curso inexistente, `403` sem
  permissão.

### 19.22 `GET /financeiro/matriculas/configuracoes`

- **Proteção:** igual à 19.21.
- **Query params:** `codigo_academia` (obrigatório para admin; forçado pelo token para academia).
- **Response 200:**
  ```json
  {
    "codigo_academia": "ACA001",
    "configuracoes": [
      {
        "codigo_academia": "ACA001",
        "nivel": "medio",
        "ano_academico": "1_ano_medio",
        "curso_id": "550e8400-e29b-41d4-a716-446655440000",
        "valor": 15000.00,
        "metodos_pagamento": ["REF", "GPO"],
        "vigente_em": "2026-08-08T12:00:00Z"
      }
    ]
  }
  ```
- **Regras de negócio:** mesma lógica de 19.15 — devolve apenas a versão mais recente de cada
  combinação `(nivel, ano_academico, curso_id)`, não um histórico completo.

### Depois de 19.22 (ordem final da seção)

A tabela `**Erros comuns das rotas autenticadas:**` que já existe hoje no fim da seção 19 continua
sendo o último elemento da seção — não a duplique, apenas garanta que ela ficou posicionada depois de
19.22. Adicione a ela (ou como uma frase logo antes dela) a **regra transversal de mapeamento de
erros** descrita na Parte G (seção 7) deste brief, porque ela se aplica a praticamente todas as rotas
novas de mensalidade e matrícula que você acabou de escrever.

---

## 6. PARTE D — Novas seções 9.9 a 9.12 (fluxo público de pagamento de matrícula)

Insira depois da atual 9.8 (`GET /academia/documentos/solicitacoes-matricula/:codigo/:campo/
download`), antes do `---` que separa a seção 9 da seção 10 (Cursos). Use o formato de 2.1 (estilo
seção 9). Adicione também as 4 novas entradas ao índice da seção 9 no topo do arquivo (ver Parte F).

Contexto a explicar em uma frase antes de 9.9 (ou dentro da introdução da seção 9, como preferir,
mas precisa aparecer): estas quatro rotas existem para o **próprio candidato**, que não tem conta no
sistema, conseguir consultar e pagar a taxa de matrícula quando a academia exige uma — sem
autenticação, usando apenas o código da solicitação. Por isso `busca` e `status` são deliberadamente
minimalistas: **posse do código da solicitação já funciona como credencial**, então essas rotas nunca
expõem documentos ou dados pessoais completos do candidato.

### 9.9 `GET /solicitacao-matricula/busca`

- **Proteção:** pública, mas limitada por IP (ver nota de rate limit abaixo).
- **Query params:** `telefone`, `telefone_encarregado`, `email`, `bilhete_identidade`,
  `bilhete_identidade_encarregado` — todos opcionais individualmente, mas **é preciso informar pelo
  menos 2 desses 5 campos, com correspondência exata** (case-sensitive, sem normalização) para a
  busca ser executada.
- **Exemplo de request:**
  ```http
  GET /solicitacao-matricula/busca?telefone=%2B244923000000&email=candidato%40example.com HTTP/1.1
  ```
- **Response 200:**
  ```json
  {
    "solicitacoes": [
      {
        "codigo_solicitacao": "AB12CD34EF5",
        "nome_estudante": "Ana Manuel",
        "academia": "ACAD001",
        "data_submissao": "2026-08-01T10:00:00Z",
        "status": "pendente"
      }
    ]
  }
  ```
- **Regras de negócio:**
  - Com menos de 2 campos preenchidos, a resposta é `200` com `"solicitacoes": []` — **não** é um
    erro `400`. Isso é deliberado: evita que a ausência/presença de erro sirva para um atacante
    confirmar se um campo isolado existe na base.
  - Só devolve dados de **reconhecimento** (código, nome, academia, data, status) — nunca documentos,
    contactos completos ou valores.
  - Ordenado por data de submissão mais recente primeiro.
- **Nota sobre limite de requisições:** esta rota tem um limitador de taxa próprio e independente das
  outras rotas públicas financeiras (inclusive independente de 9.10 e 9.11): até 5 requisições
  imediatas por IP, renovando a um ritmo de 1 a cada 3 segundos (equivalente a 20/min sustentado).
  Ao exceder, a resposta é `429` com corpo vazio (não segue o envelope padrão `{error, message,
  request_id}` da seção 1.1, porque a resposta é gerada pelo middleware de limite antes de chegar ao
  handler).

### 9.10 `GET /solicitacao-matricula/:codigo/status`

- **Proteção:** pública, com o mesmo tipo de limite de requisições de 9.9, mas em um contador
  **independente** (esgotar o limite de busca não afeta esta rota).
- **Path params:** `codigo` — código da solicitação.
- **Response 200 (solicitação não pendente de pagamento):**
  ```json
  { "status": "pendente", "codigo_academia": "ACAD001" }
  ```
- **Response 200 (solicitação aguardando pagamento de matrícula):**
  ```json
  {
    "status": "aprovada_pendente_pagamento_matricula",
    "codigo_academia": "ACAD001",
    "valor_matricula": 15000.00,
    "metodos_pagamento": ["REF", "GPO"]
  }
  ```
- **Regras de negócio:**
  - `valor_matricula` e `metodos_pagamento` só aparecem quando `status` é exatamente
    `aprovada_pendente_pagamento_matricula`; para qualquer outro status (`pendente`, `aprovada`,
    `reprovada`, `cancelada`) a resposta traz apenas `status` e `codigo_academia`.
  - Não expõe nome, documentos nem qualquer outro dado do candidato.
- **Erros comuns:** `404` quando o código não existe (usa a mesma mensagem genérica de "solicitação
  não disponível para pagamento de matrícula", mesmo sendo um caso de "não encontrada" — é
  deliberadamente genérica para não diferenciar "não existe" de "existe mas não está nesse estado"),
  `429` no limite de requisições.

### 9.11 `POST /solicitacao-matricula/:codigo/pagamento-matricula`

- **Escopo da rota:** cria a cobrança da taxa de matrícula para uma solicitação já aprovada e
  aguardando pagamento — é a única forma de o candidato efetivamente pagar.
- **Proteção:** pública, com o mesmo tipo de limite de requisições de 9.9/9.10, em contador
  independente.
- **Path params:** `codigo` — código da solicitação.
- **Request JSON:**
  ```json
  { "metodo_pagamento": "GPO", "telefone": "+244923000000" }
  ```
  `telefone` só é usado (e relevante) quando `metodo_pagamento` é `GPO`; pode ser omitido para `REF`
  e `GPO_QR`.
- **Response 201** (`MatriculaPagamentoView`):
  ```json
  {
    "cobranca": {
      "id": "76f2971c-4a7d-48f7-92c2-f8d3b28e9a2d",
      "provider_charge_id": "APPYPAY-987654",
      "merchant_transaction_id": "MATR2608LDA0001",
      "status": "pendente",
      "response": { "status": "Accepted" }
    }
  }
  ```
  Quando `metodo_pagamento` é `GPO_QR`, `cobranca` também inclui `"qrCodeArr"` em base64 (mesmo
  formato da seção 19.5); nos demais métodos o campo fica ausente.
- **Regras de negócio:**
  - A solicitação precisa estar **exatamente** no status `aprovada_pendente_pagamento_matricula`;
    qualquer outro status (incluindo `pendente`, `aprovada` sem taxa, já paga, cancelada) é recusado.
  - `metodo_pagamento` precisa estar entre os métodos congelados na aprovação daquela solicitação
    (seção 9.5) — não a configuração atual da academia, a que foi congelada no momento da aprovação.
  - **Só pode existir uma cobrança de matrícula aberta por solicitação de cada vez**; uma nova
    tentativa enquanto já existe uma cobrança em aberto (não paga, não cancelada, não falhada) é
    recusada.
  - O valor cobrado é sempre o valor **congelado no momento da aprovação** (seção 9.5), nunca o valor
    atual da configuração (que pode já ter mudado).
  - **Comportamento deliberado de resposta de erro — destaque isto:** ao contrário das rotas de
    `/financeiro/*`, que diferenciam `400`/`404`/`409`/`503` conforme a causa (ver regra transversal
    na Parte G), esta rota pública responde **sempre `409`** com a mesma mensagem genérica
    ("solicitação não disponível para pagamento de matrícula") para **qualquer** falha — solicitação
    inexistente ou em outro status, método não habilitado, cobrança já aberta, ou até uma falha
    interna inesperada. Isso é intencional: por ser uma rota pública e sem autenticação, o backend
    minimiza a informação exposta sobre o motivo exato da recusa.
  - Se a AppyPay já confirmar sucesso de forma síncrona na própria criação da cobrança (raro para
    REF/GPO, mas possível), o vínculo do estudante é efetivado imediatamente nesta mesma resposta —
    ver 9.12* (efetivação do vínculo) mais abaixo.
- **Erros comuns:** `400` payload inválido (JSON malformado), `409` para qualquer outra recusa (ver
  acima), `429` no limite de requisições.

### 9.12 `PUT /academia/solicitacao-matricula/:codigo/cancelar`

- **Escopo da rota:** a academia cancela uma solicitação que está **aguardando pagamento de
  matrícula** e nunca foi paga — por exemplo, depois de um prazo sem o candidato pagar. Esta rota
  **não** serve para cancelar uma solicitação comum ainda `pendente` (para essa, a decisão é aprovar
  ou reprovar — 9.5/9.6); ela só atua sobre o status `aprovada_pendente_pagamento_matricula`.
- **Proteção:** academia ativa, dona da solicitação.
- **Request JSON:**
  ```json
  { "motivo": "candidato não efetuou o pagamento no prazo" }
  ```
  `motivo` é obrigatório (não pode ser vazio após trim).
- **Response 200:**
  ```json
  { "message": "solicitação cancelada com sucesso" }
  ```
- **Regras de negócio:**
  - A solicitação precisa pertencer à academia autenticada (`403` caso contrário) e estar
    **exatamente** no status `aprovada_pendente_pagamento_matricula` — tentar cancelar em qualquer
    outro status responde `409` com a mensagem "somente solicitação pendente de pagamento de
    matrícula pode ser cancelada com motivo".
  - Antes de cancelar a solicitação, o backend tenta cancelar qualquer cobrança de matrícula ainda
    aberta associada a ela (mesmo mecanismo de cancelamento local da seção 19.9). **Se essa cobrança
    já tiver sido paga, o cancelamento inteiro é recusado com `400`** (mensagem "cobrança já foi paga
    e não pode ser cancelada") — a academia não consegue cancelar uma matrícula cuja taxa já entrou.
  - Se não havia nenhuma cobrança aberta (candidato nunca tentou pagar), esse passo é um no-op e o
    cancelamento da solicitação prossegue normalmente.
  - O código de estudante reservado na aprovação é liberado; nenhum `Estudante` chegou a ser criado
    neste caminho.
- **Erros comuns:** `400` motivo ausente ou cobrança já paga, `403` solicitação de outra academia,
  `404` solicitação inexistente, `409` solicitação fora do status esperado.

### Nota final da Parte D — efetivação do vínculo após pagamento (para mencionar em 9.11 e/ou como parágrafo de fechamento da seção 9)

Explique, em um parágrafo curto (pode ser no fim de 9.11 ou como um parágrafo próprio depois de
9.12), o que acontece **depois** que a AppyPay confirma o pagamento — isto conecta a seção 9 de volta
à seção 19 e evita que o leitor pense que o fluxo termina na criação da cobrança:

- A confirmação de sucesso pode chegar por três caminhos, todos convergindo para o mesmo efeito:
  consulta síncrona logo na criação da cobrança (raro), consulta manual/posterior via
  `GET /financeiro/appypay/cobrancas/:id` (seção 19.6), ou webhook AppyPay (seções 19.10/19.11).
- Em qualquer um desses três caminhos, ao detectar status `Success` para uma cobrança que tem
  `codigo_solicitacao` no seu payload, o backend cria o `Estudante` (reutilizando o código já
  reservado na aprovação) e grava `SolicitacaoMatriculaVinculada`, movendo a solicitação de volta
  para o status `aprovada` (agora com o estudante efetivamente criado e vinculado).
  - **Este é o único momento em que o `Estudante` é criado nesse fluxo com taxa** — na aprovação
    (9.5), apenas o código é reservado, sem criar o registro do estudante.
- A operação é idempotente e segura contra reentrega: se o webhook for entregue mais de uma vez, ou
  se a consulta detectar sucesso mais de uma vez, a criação do estudante só acontece na primeira vez
  — nas seguintes, a rota detecta que a solicitação já saiu do status `aprovada_pendente_pagamento_
  matricula` e não faz nada.
- Se o bilhete de identidade do candidato já tiver sido cadastrado por outra via entre a aprovação e
  a confirmação do pagamento (corrida rara), a efetivação falha e o erro é reportado — não deixe isso
  de fora: mencione que o backend reserva a unicidade do bilhete de identidade novamente,
  imediatamente antes de criar o estudante, para evitar duplicidade.

---

## 7. PARTE E — Reescrita completa da seção 9.5 (`PUT /academia/solicitacao-matricula/:codigo/aprovar`)

A seção 9.5 atual descreve **só o caminho antigo** (aprovação sempre cria o estudante na hora). Isso
está incompleto desde que a taxa de matrícula existe: hoje a aprovação tem **dois caminhos possíveis**,
dependendo de existir ou não uma configuração de taxa (seção 19.21) para a combinação nível/ano/curso
da solicitação. Reescreva a seção inteira (mantendo o número `9.5` e a rota) cobrindo os dois casos.
Não precisa preservar o texto atual literalmente, mas **não perca nenhuma regra que já está lá
hoje** (revalidação de documentos, reserva do BI, geração do código do estudante, cancelamento de
solicitações concorrentes por BI, etc. — tudo isso continua válido e acontece **antes** de o fluxo se
dividir em dois).

Estrutura sugerida (pode ajustar a redação, mas cubra exatamente estes pontos):

**Proteção, path params:** mantenha como já está hoje (academia ativa; sem corpo de request).

**Regras de negócio — parte comum aos dois caminhos** (igual ao texto atual, mantenha):

- Solicitação deve existir, pertencer à academia autenticada e estar `pendente`.
- Documentos e BI do encarregado são revalidados no momento da aprovação.
- Se a solicitação tem `bilhete_identidade`, a unicidade é reservada antes de prosseguir; conflito
  de BI responde `409`.
- É gerado `codigo_estudante` (sempre, nos dois caminhos — mesmo no caminho com taxa, o código é
  gerado e reservado imediatamente, apenas o `Estudante` em si é adiado).

**Caminho 1 — sem taxa configurada (comportamento padrão, igual ao que já existia):**

- Quando não existe nenhuma configuração de taxa de matrícula (seção 19.21) para o nível/ano/curso da
  solicitação, a aprovação **cria o `Estudante` imediatamente**, grava `SolicitacaoMatriculaAprovada`
  e o estudante já nasce vinculado à academia com senha inicial padrão.
- Mantenha a explicação já existente sobre cancelamento de solicitações concorrentes pendentes com o
  mesmo BI (`cancelarSolicitacoesConcorrentes`).
- **Response 200** (igual à atual):
  ```json
  {
    "message": "solicitação aprovada e estudante registado com sucesso",
    "codigo_solicitacao": "AB12CD34EF5",
    "codigo_estudante_gerado": "EST123456"
  }
  ```

**Caminho 2 — com taxa configurada:**

- Quando existe uma configuração de taxa vigente para o nível/ano/curso da solicitação (resolvida
  pela mesma lógica de vigência por data da seção 19.21), a aprovação **não cria o `Estudante`**: ela
  grava `SolicitacaoMatriculaAprovada` seguido imediatamente de
  `SolicitacaoMatriculaAprovadaPendentePagamento`, e o status final da solicitação passa a ser
  `aprovada_pendente_pagamento_matricula`.
- O valor e os métodos de pagamento da configuração vigente **no momento da aprovação** ficam
  congelados nesta solicitação (gravados nela) — mudanças futuras na configuração não afetam
  solicitações já aprovadas.
- O código do estudante já foi gerado e fica reservado nesta solicitação, mas **nenhum registro de
  `Estudante` existe ainda** — ele só é criado quando o pagamento é confirmado (ver nota de
  efetivação de vínculo, no final da Parte D deste brief) ou quando a academia cancela a solicitação
  pendente de pagamento (seção 9.12).
- Cancelamento de solicitações concorrentes por BI **não acontece neste caminho** (só ocorre no
  caminho 1, quando o estudante é criado de fato).
- **Response 200:**
  ```json
  {
    "message": "solicitação aprovada, aguardando pagamento de matrícula",
    "codigo_solicitacao": "AB12CD34EF5",
    "codigo_estudante_gerado": "EST123456",
    "status": "aprovada_pendente_pagamento_matricula"
  }
  ```

**Erros comuns:** mantenha a lista atual (`400`, `403`, `404`, `409`) — ela já cobre os dois
caminhos, já que a bifurcação acontece depois de todas as validações que geram esses erros.

Adicione uma frase de fechamento com referência cruzada: "Para o fluxo de consulta e pagamento da
taxa pelo próprio candidato, veja as seções 9.9–9.12; para a configuração da taxa pela academia, veja
a seção 19.21."

---

## 8. PARTE F — Atualização do Índice (topo do arquivo)

No bloco `## Índice`, dentro do item `9. [Solicitação de Matrícula]`, depois da linha do link para
9.8, adicione (mesmo formato dos itens 9.1–9.8 já existentes):

```
   - [9.9 `GET /solicitacao-matricula/busca`](#99-get-solicitacao-matriculabusca)
   - [9.10 `GET /solicitacao-matricula/:codigo/status`](#910-get-solicitacao-matriculacodigostatus)
   - [9.11 `POST /solicitacao-matricula/:codigo/pagamento-matricula`](#911-post-solicitacao-matriculacodigopagamento-matricula)
   - [9.12 `PUT /academia/solicitacao-matricula/:codigo/cancelar`](#912-put-academiasolicitacao-matriculacodigocancelar)
```

Gere os anchors (`#9x-...`) da mesma forma que os existentes 9.1–9.8 foram gerados (título em
minúsculas, espaços e `/` viram `-`, `:` e crases são removidos) — confira contra os anchors já
existentes no índice atual para replicar exatamente o mesmo padrão de slug.

A entrada de nível 1 para a seção 19 (`19. [Financeiro / AppyPay](#19-financeiro--appypay)`)
**não precisa listar subitens** — hoje ela não lista (mesmo com 19.1–19.14 existindo), então mantenha
esse padrão e não adicione uma lista de 19.1–19.22 ali, para não destoar do estilo já usado pelo
próprio documento nessa seção.

---

## 9. PARTE G — Regra transversal de mapeamento de erros do módulo financeiro

Esta é uma clarificação importante que descobrimos ao revisar o código e que **não está explícita em
nenhum lugar do documento hoje**, embora afete a leitura correta de quase todo endpoint financeiro
novo ou já existente. Adicione-a como um parágrafo (ou uma linha extra na tabela) logo antes da
tabela `**Erros comuns das rotas autenticadas:**`, no final da seção 19 (depois de 19.22):

> Nas rotas de `/financeiro/*` (exceto o fluxo público da seção 9.9–9.12, que tem sua própria regra
> — ver 9.11), o mapeamento de erro para código HTTP segue um princípio único: `404` só ocorre para
> um recurso *referenciado* que não existe (academia, curso, cobrança ou credencial informados que
> não são encontrados); `409` só ocorre para uma operação equivalente já em processamento por
> idempotência (mesmo `merchantTransactionId` ainda sendo processado); `503` só ocorre para falha de
> comunicação com a AppyPay. **Qualquer outra violação de regra de negócio — incluindo mensagens que
> descrevem um "conflito" em português, como "mensalidade já possui cobrança em aberto" ou
> "cobrança já foi paga e não pode ser cancelada" — responde `400`.** Não infira `409` a partir do
> conteúdo da mensagem de erro; apenas os dois casos específicos acima (idempotência de
> `merchantTransactionId`, e as transições de estado de aggregate fora do módulo financeiro, como
> aprovar/reprovar/cancelar solicitações — seções 9.5, 9.6, 9.12) usam `409`.

Ao escrever os blocos **Erros comuns** de cada novo endpoint das Partes B e C, siga rigorosamente
esse princípio (os exemplos de código HTTP já indicados em cada bloco acima já seguem essa regra —
apenas garanta consistência ao redigir o texto final).

---

## 10. PARTE H — Atualização da seção `### 2.5 SolicitacaoMatricula`

A interface TypeScript `SolicitacaoMatriculaDTO` de hoje está incompleta em três pontos. Atualize-a
para refletir exatamente os campos abaixo (todos confirmados no `SolicitacaoMatriculaDTO` real do
backend, em `internal/projections/solicitacao_matricula_projection.go`):

1. Adicione `telefone_encarregado?: string` logo depois de `telefone?: string` (esse campo já existe
   no backend e é usado nos exemplos de 9.1 — só faltava na interface documentada).
2. Adicione, no final da interface, os dois campos novos do fluxo de taxa de matrícula:
   ```typescript
   valor_matricula?: number
   metodos_pagamento_matricula?: string[]
   ```
   (equivalentes a `valor_matricula`/`metodos_pagamento_matricula`, presentes só quando a
   solicitação passou ou está passando pelo caminho 2 da seção 9.5).
3. Atualize o `type SolicitacaoMatriculaStatus` na seção `2.1 Tipos Base` (não em 2.5 — é lá que o
   tipo está definido) para incluir o novo valor:
   ```typescript
   type SolicitacaoMatriculaStatus = 'pendente' | 'aprovada' | 'reprovada' | 'cancelada' | 'aprovada_pendente_pagamento_matricula'
   ```

Não altere mais nada nessas duas interfaces além do listado acima.

---

## 11. PARTE I — Remover a seção órfã final

Depois de concluir as Partes B a E, **todo o conteúdo relevante** que hoje está na seção solta
`## Cobrança de matrícula por solicitação` (a última seção do arquivo, depois de `## 21. Integrações
Externas / Ziett (Teste)`) já terá sido reescrito de forma mais completa dentro das seções 9.5,
9.9–9.12 e 19.21–19.22. **Apague essa seção inteira** (do cabeçalho `## Cobrança de matrícula por
solicitação` até o fim do arquivo) — ela não deve mais existir depois desta tarefa.

Antes de apagar, faça uma checagem simples: releia esse parágrafo uma última vez e confirme que cada
frase dele tem um "lar" correspondente e mais completo em algum dos blocos novos que você acabou de
escrever. Se encontrar algum detalhe ali que não tenha sido coberto em nenhuma das Partes B–E, **não
o descarte** — incorpore-o no bloco mais apropriado antes de apagar a seção.

---

## 12. Checklist de autoverificação (rodar antes de finalizar, sem precisar de BD/Docker)

1. `grep -n "^#### 19\." "Documentação da API.md"` — confirme que a sequência é 19.1 a 19.22, sem
   pulos nem repetições.
2. `grep -n "^### 9\." "Documentação da API.md"` (dentro da seção 9) — confirme 9.1 a 9.12
   sequenciais.
3. `grep -n "seção 19\." "Documentação da API.md"` — releia cada ocorrência e confirme que o número
   citado corresponde ao endpoint correto na nova numeração (compare com a tabela da Parte A, seção
   3.1).
4. `grep -n "## Cobrança de matrícula por solicitação"` — deve retornar **vazio** (a seção foi
   removida).
5. Confirme que todo bloco ```` ```json ```` tem chaves/colchetes balanceados (abra e leia cada
   exemplo novo).
6. Confirme que os 4 novos anchors do índice (9.9–9.12) batem com os cabeçalhos reais criados (mesma
   grafia, mesmo slug).
7. Confirme que a tabela de rotas no topo da seção 19 agora tem as 3 linhas de
   `/financeiro/matriculas/configuracoes`.
8. Releia a seção 2.5 e o `type SolicitacaoMatriculaStatus` em 2.1 e confirme os 4 campos/valor
   adicionados (Parte H).

## 13. Fora de escopo — não faça

- Não altere nenhum arquivo `.go`.
- Não altere nenhuma seção do Markdown fora das listadas na Parte 1 deste brief, mesmo que perceba
  outros problemas de documentação enquanto trabalha (ex.: não mexa nas seções 6, 8, 10 etc.).
- Não tente rodar o servidor, migrações ou testes.
- Não renumere nada além do que está explicitamente listado na Parte A.

## 14. Entrega

Ao terminar, liste em texto simples (fora do arquivo `.md`, como resposta da sua execução):

- Confirmação de que o checklist da seção 12 foi executado e passou.
- A lista final de números de seção criados/renumerados (ex.: "19.9–19.13 renumerados; 19.14–19.22
  novos; 9.9–9.12 novos; 9.5 reescrita; 2.1 e 2.5 atualizadas; seção órfã final removida").
- Qualquer ponto em que você teve que tomar uma decisão de redação não coberta explicitamente por
  este brief (para eu revisar depois).
