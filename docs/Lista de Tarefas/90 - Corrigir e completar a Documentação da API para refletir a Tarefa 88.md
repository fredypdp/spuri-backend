---
criado: 07-09-2026
origem: Fredy + Claude (auditoria pós-Tarefa 88)
status: pendente
tipo: documentação (spuri-backend)
depende_de: Tarefa 88 (implementada) e Tarefa 89 (correção crítica — deve estar aplicada, pois este
  documento descreve o comportamento final já com ela)
---

# Corrigir e completar a `Documentação da API.md` para refletir a Tarefa 88

### Documento de execução para o Codex (orquestrado e pré-testado pelo Claude)

> **Este documento já contém todas as decisões e fatos necessários.** Você (Codex) não precisa
> investigar o "porquê" de nenhuma regra nem decidir formato — apenas **escrever o texto final**
> seguindo exatamente as instruções abaixo, no arquivo `Documentação da API.md` (raiz do repositório).
> Todos os fatos de negócio abaixo foram extraídos diretamente do código-fonte Go (handlers,
> aggregates, projections, migrations) por outra instância de IA (Claude), que revisou cada arquivo
> relevante e testou as regras de negócio contra uma base PostgreSQL 16 real antes de escrever este
> brief. Não é necessário reler o código para validar os fatos de negócio descritos aqui.

## 0. Leia isto primeiro — sobre o seu ambiente e o que NÃO é necessário

- Esta tarefa é **exclusivamente de documentação**: um único arquivo Markdown (`Documentação da
  API.md`) é editado. **Nenhum arquivo `.go` deve ser alterado.**
- **Não é necessário** banco de dados, Docker, `psql`, `go build`, `go test` ou subir o servidor —
  nada nesta tarefa depende disso.
- Autovalidação permitida/esperada (sem infraestrutura): conferir que blocos de código JSON estão bem
  formados, que não sobraram números de seção duplicados/quebrados, que os links do índice continuam
  batendo com os títulos, e que a numeração ficou sequencial. Isso pode ser feito só lendo o Markdown.

## 1. Prompt recomendado para executar esta tarefa

> Aplique exatamente as duas edições descritas neste documento em `Documentação da API.md`: (1)
> adicionar uma entrada no Índice para a nova seção 23, e (2) substituir todo o bloco final do arquivo
> (da linha que começa com `## 20. Serviços Extras` até o fim do arquivo) pelo texto completo fornecido
> na seção 5. As decisões de redação já estão tomadas — não replaneje, não invente novos rótulos de
> subtítulo, e não toque em nenhuma outra seção do documento. Ao final, rode a checklist da seção 6.

## 2. Por que isto é mais do que só "atualizar os campos da Tarefa 88"

Ao auditar a Tarefa 88, descobri que a seção "Serviços Extras" da documentação já tinha um problema
**anterior e independente** da Tarefa 88: ela está com numeração de seção duplicada e quebrada, e nunca
apareceu no Índice do topo do arquivo. Como a Tarefa 88 exige editar exatamente essa seção, não dá para
fazer isso direito sem também consertar a numeração — senão o problema só cresce.

Evidência (`grep -n "^## " "Documentação da API.md"`, trecho relevante):

```
8724:## 20. Armazenamento
8998:## 21. Integrações Externas / Ziett (Teste)
9067:## 22. Sumários
9200:## 20. Serviços Extras          ← duplica o número 20 (já usado por "Armazenamento" acima)
```

E dentro desse bloco final (`grep -n "^### 19\|^### 20" "Documentação da API.md"`, trecho relevante):

```
8525:#### 19.20 POST /financeiro/mensalidades/obrigacoes/reativar     ← seção 19 real, intocada
9224:### 19.20 Pagamento de taxa de inscrição de serviço extra        ← reusa "19.20" indevidamente
9227:### 20. Serviços Extras — inscrições                              ← título de nível 3 mas com "20."
9230:### 20.7 Pendências e pagamento                                   ← continua a numeração indevida
```

Confirmei (`grep -n "eção 20\|ecção 20\|19\.20"`) que **nada mais no documento referencia esses números
por nome** — ou seja, renumerar aqui é seguro, sem quebrar links cruzados em outras seções.

**Decisão de redação:** todo o bloco final (hoje mal numerado como "20.1"–"20.5", "19.20" solto, "20.
Serviços Extras — inscrições" e "20.7") vira uma única seção nova e correta, `## 23. Serviços Extras`
(depois de "22. Sumários", que é a última seção real do índice hoje), com subseções `23.1` a `23.9`
sequenciais. Nenhum parágrafo de conteúdo é reescrito além do que a Tarefa 88 exige — é
essencialmente uma renumeração, mais a inserção do conteúdo novo da Tarefa 88 (categoria_servico_id,
detalhes_personalizados tipado, CRUD de categorias) no meio dela.

## 3. Escopo exato — só toque nisto

Arquivo: `Documentação da API.md` (raiz do repo).

Seções que **serão editadas**:

- Índice (topo do arquivo) — uma linha nova adicionada depois do item 22.
- Todo o bloco que vai da linha `## 20. Serviços Extras` (hoje por volta da linha 9200) até o final do
  arquivo — é **literalmente o final do arquivo** (confirmado: o arquivo tem 9234 linhas no total, e
  esse bloco vai da linha 9200 até a 9234). Substitua esse bloco inteiro pelo texto da seção 5.

**Não toque em mais nada.** Em especial, não mexa nas seções 19 (Financeiro/AppyPay), 20
(Armazenamento — a seção "20." **real e correta**, que fica ANTES do bloco duplicado), 21 ou 22.

## 4. Localizar/substituir — Índice

### 4.1 Localizar

```
22. [Sumários](#22-sumários)

---

## 1. Convenções Globais
```

### 4.2 Substituir por

```
22. [Sumários](#22-sumários)
23. [Serviços Extras](#23-serviços-extras)

---

## 1. Convenções Globais
```

(Mesmo estilo simples de linha única já usado pelos itens 19–22 do índice — sem sub-bullets, já que
19–22 também não têm.)

## 5. Localizar/substituir — bloco final do arquivo

### 5.1 Localizar

Da linha `## 20. Serviços Extras` até o final do arquivo (a última linha do arquivo hoje termina em
"...administram uma obrigação individual da inscrição."). Se preferir confirmar por conta própria antes
de substituir, rode `grep -n "^## 20. Serviços Extras" "Documentação da API.md"` — deve haver
exatamente uma ocorrência, e tudo dali em diante até o EOF é o bloco a substituir.

### 5.2 Substituir por

(A cerca de código abaixo usa QUATRO crases só porque o conteúdo de dentro já tem blocos ```json — as
quatro crases são apenas o delimitador deste documento de tarefa, não devem ser copiadas.)

````markdown
## 23. Serviços Extras

A academia pode configurar serviços adicionais, como transporte e atividades extracurriculares. Serviços pagos ou com taxa de inscrição exigem credenciais AppyPay configuradas.

### 23.1 Criar serviço extra
**Proteção:** academia autenticada e ativa. `POST /academia/servicos-extras`.

**Request body:** `nome` (obrigatório), `descricao`, `categoria_servico_id`, `pago`, `preco`, `tipo_cobranca` (`unico` ou `mensal`), `metodos_pagamento` (`GPO`, `REF`, `GPO_QR`), `tem_taxa_inscricao`, `valor_taxa_inscricao`, `metodos_pagamento_taxa_inscricao`, `anos_academicos_disponiveis`, `cursos_disponiveis`, `documento_obrigatorio`, `documento_instrucoes` e `detalhes_personalizados`.

**`categoria_servico_id`:** opcional (UUID). Quando informado, precisa existir, pertencer à mesma academia autenticada e estar `ativo=true` no momento da criação — caso contrário `400` (`categoria de serviço não encontrada`, `categoria de serviço não pertence a esta academia` ou `categoria de serviço está inativa`, conforme o caso). Ver seção 23.6 para o CRUD de categorias. Uma vez associada, a categoria continua valendo mesmo que seja desativada depois — ela só deixa de poder ser escolhida em serviços NOVOS ou em atualizações que a troquem.

**`detalhes_personalizados`:** objeto com até 30 chaves, onde cada chave mapeia para `{ "rotulo": string, "valor": ..., "tipo": string }`:

- `rotulo`: obrigatório, até 100 caracteres — o texto exibido no formulário.
- `tipo`: um de `texto`, `numero`, `booleano`, `data`, `hora`, `lista_texto`.
- `valor`: formato depende de `tipo` — `texto` é string; `numero` é número; `booleano` é `true`/`false`; `data` é string `"AAAA-MM-DD"`; `hora` é string `"HH:MM"`; `lista_texto` é array de strings.
- Cada chave deve seguir o formato `snake_case` (minúsculas, números e `_`, começando por letra, até 50 caracteres).

Exemplo (`Transporte Escolar`):
```json
{
  "nome": "Transporte Escolar",
  "categoria_servico_id": "3f2a1c4e-7b1a-4e9d-9c2a-1a2b3c4d5e6f",
  "pago": false,
  "detalhes_personalizados": {
    "rota": { "rotulo": "Rota", "valor": "Centro", "tipo": "texto" },
    "ponto_de_embarque": { "rotulo": "Ponto de embarque", "valor": "Escola", "tipo": "texto" },
    "horario_de_saida": { "rotulo": "Horário de saída", "valor": "06:30", "tipo": "hora" }
  }
}
```

Exemplo (`Natação`):
```json
{
  "nome": "Natação",
  "pago": true,
  "preco": 25000.00,
  "tipo_cobranca": "mensal",
  "metodos_pagamento": ["GPO"],
  "detalhes_personalizados": {
    "piscina": { "rotulo": "Piscina", "valor": "Olímpica", "tipo": "texto" },
    "exige_saber_nadar": { "rotulo": "Exige saber nadar", "valor": true, "tipo": "booleano" },
    "equipamento_incluido": { "rotulo": "Equipamento incluído", "valor": ["touca", "óculos"], "tipo": "lista_texto" }
  }
}
```

**Regras de negócio:** campos financeiros são obrigatórios apenas quando a respectiva cobrança estiver ativa; `anos_academicos_disponiveis` e `cursos_disponiveis` vazios (ambos) disponibilizam o serviço para todos os anos/cursos. `anos_academicos_disponiveis` só aceita anos de ensino fundamental (`N_ano_fundamental`), sem vínculo com curso — o ensino fundamental não tem cursos neste sistema. `cursos_disponiveis` restringe a um curso específico: cada item é `"<curso_id>|<ano_academico>"`, com ano médio ou superior — médio e superior são sempre escopados a um curso, nunca soltos. O curso precisa pertencer à mesma academia, não estar deletado, ter o tipo correspondente ao ano e conter o ano entre seus anos acadêmicos. As duas listas são combináveis (ex.: fundamental solto + um curso médio específico). Em `POST /estudante/servicos-extras/:id/solicitacao`, se o serviço tiver restrições, o estudante só consegue se inscrever quando seu ano/curso atual corresponder a uma das listas; caso contrário recebe `403`.

### 23.2 Atualizar serviço extra
**Proteção:** academia proprietária autenticada e ativa. `PUT /academia/servicos-extras/:id`. Aceita os mesmos campos da criação (incluindo `cursos_disponiveis` e `categoria_servico_id`) de forma parcial — envie só os campos que quer alterar. Enviar `categoria_servico_id` explicitamente como `null` remove a categoria do serviço; omitir o campo mantém a categoria atual inalterada.

### 23.3 Desativar serviço extra
**Proteção:** academia proprietária autenticada e ativa. `PUT /academia/servicos-extras/:id/desativar`.

### 23.4 Reativar serviço extra
**Proteção:** academia proprietária autenticada e ativa. `PUT /academia/servicos-extras/:id/reativar`.

### 23.5 Listar e consultar serviços extras
**Proteção:** `GET /academia/servicos-extras` e `GET /academia/servicos-extras/:id` exigem academia ou admin autenticado. A listagem pública `GET /academia/servico/:codigo_academia/servicos-extras` retorna somente serviços ativos.

### 23.6 Categorias de serviço
**Proteção:** todas exigem academia autenticada e ativa; as rotas de escrita (atualizar/desativar/reativar) exigem que a categoria pertença à academia autenticada — `403` caso contrário (`404` se o ID não existir).

- `POST /academia/categorias-servico` cria uma categoria. **Request body:** `nome` (obrigatório, até 100 caracteres).
- `PUT /academia/categorias-servico/:id` renomeia. Mesmo corpo da criação.
- `PUT /academia/categorias-servico/:id/desativar` e `PUT /academia/categorias-servico/:id/reativar` alternam o status.
- `GET /academia/categorias-servico` lista as categorias da academia autenticada; aceita `?ativos=true` para retornar somente as ativas.

**Regras de negócio:** nome único por academia, ignorando maiúsculas/minúsculas, enquanto a categoria estiver ativa — duas categorias ativas com o "mesmo" nome (case-insensitive) não podem coexistir na mesma academia, seja por criação, renomeação ou reativação; a tentativa é rejeitada com `400` (`já existe uma categoria de serviço ativa com este nome nesta academia`). Ao desativar uma categoria, o nome fica livre para reutilização por outra categoria (nova ou reativada). Desativar uma categoria não afeta os serviços que já a utilizam — eles continuam funcionando normalmente; a categoria só deixa de poder ser escolhida em serviços novos ou em atualizações de `categoria_servico_id`.

### 23.7 Pagamento de taxa de inscrição de serviço extra
`POST /financeiro/servicos-extras/taxa-inscricao/pagamento?solicitacao_id={uuid}` (estudante autenticado) inicia o pagamento da taxa já aprovada. O corpo aceita `metodo_pagamento` e, para GPO, `telefone`.

### 23.8 Inscrições em serviços extras
Estudantes podem criar solicitações em `POST /estudante/servicos-extras/:id/solicitacao`, consultar `GET /estudante/servicos-extras/minhas-inscricoes` e cancelar vínculos próprios. Academias listam, aprovam, reprovam ou cancelam solicitações em `/academia/servicos-extras/solicitacoes` e `/academia/servicos-extras/inscricoes/:id/cancelar`. As listagens financeiras aceitam `origem=servico_extra` para filtrar exclusivamente taxas de inscrição de serviços extras. Uma inscrição pode ter cobranças de `taxa_inscricao`, `mensalidade` ou `preco_unico`, sempre com `origem=servico_extra`.

### 23.9 Pendências e pagamento
- `GET /estudante/servicos-extras/minhas-inscricoes/:id/pendencias` lista as pendências da própria inscrição.
- `GET /academia/servicos-extras/inscricoes/:id/pendencias` oferece a visão da academia.
- `POST /financeiro/servicos-extras/obrigacao/pagamento` inicia o pagamento de uma mensalidade ou preço único.
- `POST /financeiro/servicos-extras/obrigacao/anular` e `/reativar` administram uma obrigação individual da inscrição.
````

**Atenção ao colar:** o bloco acima está entre crases triplas só para delimitar onde ele começa/termina
neste documento de tarefa — ao colar no arquivo real, **não inclua as crases triplas externas**, apenas
o conteúdo Markdown de dentro (que por sua vez já contém, corretamente, blocos ` ```json ` internos que
DEVEM ser preservados).

## 6. Checklist de autoverificação (rodar antes de finalizar, sem precisar de BD/Docker)

1. `grep -n "^## 20\." "Documentação da API.md"` — deve retornar **só uma vez**, para "Armazenamento".
2. `grep -n "^## 23\." "Documentação da API.md"` — deve retornar **exatamente uma vez**, "Serviços Extras".
3. `grep -n "^### 23\." "Documentação da API.md"` — confirme 23.1 a 23.9, sequenciais, sem pulos nem repetições.
4. `grep -n "19\.20" "Documentação da API.md"` — deve retornar **só uma vez** agora (a rota real de mensalidades, `#### 19.20 POST /financeiro/mensalidades/obrigacoes/reativar`); a ocorrência indevida em "Serviços Extras" não deve mais existir.
5. `grep -n "categoria\"" "Documentação da API.md"` na seção 23 — não deve sobrar nenhuma menção ao campo antigo `categoria` (texto livre); só `categoria_servico_id`.
6. Confirme que todo bloco ` ```json ` novo tem chaves/colchetes balanceados (releia os dois exemplos).
7. Confirme que a nova linha do Índice (`23. [Serviços Extras](#23-serviços-extras)`) bate com o slug
   real gerado pelo cabeçalho `## 23. Serviços Extras` (mesmo padrão de acentuação/hífens já usado
   pelas entradas 19–22 do índice).
8. Confirme visualmente que o arquivo termina exatamente na linha
   `- \`POST /financeiro/servicos-extras/obrigacao/anular\` e \`/reativar\` administram uma obrigação individual da inscrição.`
   sem conteúdo duplicado ou sobrando depois.

## 7. Fora de escopo — não faça

- Não altere nenhum arquivo `.go`.
- Não altere nenhuma outra seção do Markdown além do Índice (item 4) e do bloco final (item 5), mesmo
  que perceba outros problemas de documentação em outras partes do arquivo enquanto trabalha.
- Não reordene os parágrafos dentro do bloco (ex.: mover "Inscrições" para antes de "Pagamento de taxa
  de inscrição") — a única mudança de estrutura aprovada é a renumeração e a inserção do conteúdo novo
  da Tarefa 88/89 nos pontos já indicados.
- Não tente rodar o servidor, migrações ou testes.

## 8. Entrega

Ao terminar, liste em texto simples (fora do arquivo `.md`, como resposta da sua execução):

- Confirmação de que a checklist da seção 6 foi executada e passou.
- Confirmação do número final de linhas do arquivo (`wc -l "Documentação da API.md"`).
- Qualquer ponto em que você teve que tomar uma decisão de redação não coberta explicitamente por este
  brief (para eu revisar depois).
