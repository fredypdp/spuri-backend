---
criado: 2026-07-18 00:00
origem: Lista de tarefas.md
status: pendente
---

# Implementar gabarito de prova com digitalização e correção automática (pendente)

## Prompt recomendado para executar a atualização

Implemente a atualização descrita neste documento criando a entidade `Gabarito`, vinculada a matéria, categoria de nota, ano letivo, ano acadêmico e período, com as respostas corretas de uma prova objetiva. Em seguida, crie o fluxo de digitalização da folha de respostas de um estudante e a comparação automática contra o gabarito para calcular a nota. Toda nota criada a partir desse mecanismo deve carregar `gabarito_id`, permitindo rastrear qual gabarito originou aquela nota. Divida a implementação em duas fases, conforme descrito abaixo, para tornar o escopo tecnicamente tratável: a Fase 1 entrega o modelo de dados e a comparação/cálculo sem depender de reconhecimento óptico; a Fase 2 adiciona a digitalização/OCR da folha de respostas física. Ao final de cada fase, atualize testes, documentação técnica, OpenAPI/Swagger e qualquer documentação afetada. Não criar suporte a regras antigas, aliases, wrappers de compatibilidade ou fallbacks temporários.

## Contexto

`Lista de tarefas.md` propõe: "Criar uma funcionalidade de colocar o gabarito/chave de uma prova, somado com a funcionalidade de digitalização de documentos poderá ser feita a comparação da correção da prova e o gabarito/chave. As notas serão sempre atreladas (pelo id) a um gabarito/chave." Esta é uma funcionalidade nova, sem equivalente hoje no sistema, e envolve duas partes de complexidade muito diferente:

1. um **modelo de dados** de gabarito (respostas corretas por questão) e a **lógica de comparação/pontuação** contra as respostas de um estudante — isso é tratável com as mesmas ferramentas já usadas no resto do backend;
2. a **digitalização/reconhecimento óptico** de uma folha de respostas física (foto ou scan) — isso exige avaliação de bibliotecas/serviços de OCR/visão computacional, uma decisão técnica de peso semelhante à já feita para a escolha da biblioteca Mega (`docs/Tarefas feitas/Substituir MEGAcmd por go-mega no storage Mega.md`), e carrega risco real de erro de leitura, que pode gerar nota incorreta se não houver revisão humana antes da gravação definitiva.

Por isso, esta tarefa separa as duas partes em fases: a Fase 1 entrega valor imediato (gabarito + comparação, com entrada de respostas ainda manual/estruturada) sem depender de OCR; a Fase 2 adiciona a digitalização por cima do modelo já pronto da Fase 1, sem precisar redesenhar o restante do sistema.

Dado o risco de erro de leitura por OCR, o fluxo de digitalização não deve criar a nota automaticamente sem revisão: o resultado extraído deve ser apresentado para confirmação da academia antes de virar `NotaDTO`, seguindo o mesmo espírito de "revisão humana antes de ação irreversível" já usado em outras partes sensíveis do sistema (ex.: aprovação de solicitação de matrícula).

## Resumo executivo

| Item | Decisão | Resultado esperado |
| --- | --- | --- |
| Fase 1 | Entidade `Gabarito` + comparação/pontuação com respostas informadas de forma estruturada | Cálculo automático de nota a partir de gabarito, sem depender de OCR |
| Fase 2 | Digitalização/OCR da folha de respostas física | Extração automática das respostas marcadas, com revisão obrigatória antes da nota ser criada |
| Rastreabilidade | `NotaDTO` ganha campo opcional `gabarito_id` | Toda nota originada de correção automática é rastreável até o gabarito usado |
| Revisão humana | Resultado da correção automática não vira nota sem confirmação | Erros de leitura não geram nota incorreta silenciosamente |
| Escala de nota | Reaproveitar a escala já validada por ano acadêmico | Nenhuma divergência de escala entre correção automática e lançamento manual |

---

# 1. Fase 1 — Entidade `Gabarito` e comparação/pontuação

## Objetivo

Criar o modelo de dados do gabarito e a lógica de comparação/pontuação contra as respostas de um estudante, sem depender de digitalização.

## Regra de negócio

### 1.1 Campos de `Gabarito`

| Campo | Tipo | Obrigatório | Descrição |
| --- | --- | --- | --- |
| `id` | UUID | Sim (gerado) | |
| `codigo_academia` | string | Sim | |
| `materia_disciplinar_id` | UUID | Sim | Matéria à qual a prova pertence |
| `categoria` | string | Sim | Categoria de nota (mesma validação já usada em `POST /academia/notas-aluno`: catálogo fixo para escolas, categoria configurada pela academia para o Superior) |
| `ano_lectivo` | string | Sim | Formato `YYYY_YYYY` |
| `ano_academico` | string | Sim | Deve ser compatível com a matéria informada |
| `periodo` | string | Sim | Trimestre ou semestre, conforme o tipo de ensino da matéria |
| `quantidade_questoes` | int | Sim | Total de questões da prova |
| `respostas_corretas` | map | Sim | Mapa `número da questão → resposta correta` (ex.: `{"1": "A", "2": "C"}`) |
| `peso_por_questao` | número | Não | Peso de cada questão; se omitido, todas as questões têm peso igual |
| `status` | enum | Sim (gerado) | `ativo` \| `inativo` |
| `created_at`/`version` | | Sim (gerado) | |

### 1.2 Criação de gabarito

Criar `POST /academia/gabarito`, protegido por autenticação de academia ativa, validando:

1. `materia_disciplinar_id` pertence à academia, está ativa e é compatível com `ano_academico`/`periodo` informados, reaproveitando as mesmas validações já usadas em `POST /academia/notas-aluno`;
2. `categoria` é válida para o `ano_academico`/tipo de ensino, reaproveitando exatamente a mesma validação de categoria já usada no lançamento de notas;
3. `respostas_corretas` cobre exatamente as questões de `1` até `quantidade_questoes`, sem lacunas nem duplicidade;
4. cada resposta em `respostas_corretas` está dentro de um conjunto de alternativas válidas (a definir pela implementação, ex.: `A`–`E`).

`GET /academia/gabaritos` e `GET /academia/gabarito/:id` para consulta.

### 1.3 Registro de respostas do estudante e cálculo da nota

Criar `POST /academia/gabarito/:id/correcao`, aceitando as respostas do estudante já estruturadas (ex.: `{"codigo_estudante": "ABC1234", "respostas": {"1": "A", "2": "B"}}`), sem exigir arquivo digitalizado nesta fase. O backend deve:

1. comparar `respostas` contra `respostas_corretas` do gabarito, questão a questão;
2. calcular o número de acertos (ponderado por `peso_por_questao`, se configurado);
3. converter o resultado para a escala de nota já validada para o `ano_academico` da matéria (`0–10` ou `0–20`, conforme regra já existente em `POST /academia/notas-aluno`);
4. criar a `Nota` correspondente (mesma validação de duplicidade, período e categoria já existente), incluindo o novo campo `gabarito_id` apontando para o gabarito usado.

### 1.4 Campo `gabarito_id` em `NotaDTO`

Adicionar `gabarito_id?: string` (opcional) a `NotaDTO`, preenchido apenas quando a nota se originar deste mecanismo. Notas lançadas manualmente continuam com `gabarito_id` ausente/nulo.

### 1.5 Testes obrigatórios

1. criação de gabarito válido: sucesso;
2. criação de gabarito com `respostas_corretas` incompleta ou com questão duplicada: rejeitado;
3. criação de gabarito com `categoria` inválida para o `ano_academico`: rejeitado, reaproveitando a mensagem já usada em `POST /academia/notas-aluno`;
4. correção de prova com todas as respostas certas: nota máxima da escala aplicável;
5. correção de prova com acertos parciais: nota proporcional calculada corretamente, incluindo peso por questão quando configurado;
6. nota criada por este fluxo possui `gabarito_id` preenchido;
7. duplicidade de nota (mesma combinação academia/ano/período/matéria/tipo/categoria) continua bloqueada mesmo quando a nota é originada por gabarito.

---

# 2. Fase 2 — Digitalização e reconhecimento da folha de respostas

## Objetivo

Permitir que a academia envie uma imagem ou PDF da folha de respostas física de um estudante e obtenha as respostas marcadas extraídas automaticamente, sujeitas a revisão antes de virarem nota.

## Regra de negócio

### 2.1 Avaliação técnica prévia obrigatória

Antes de implementar código, avaliar e documentar (seguindo o mesmo padrão de decisão técnica já usado para a escolha da biblioteca Mega) as opções de reconhecimento óptico disponíveis para o ambiente do backend, considerando:

- suporte a leitura de marcações em folha de respostas de múltipla escolha (não é reconhecimento de texto livre);
- possibilidade de rodar localmente vs. dependência de serviço externo;
- custo operacional e limites de uso, se for serviço externo;
- facilidade de testes automatizados sem depender de credenciais externas (equivalente ao padrão já usado de "provider local" nos testes de storage).

Registrar a decisão tomada e as limitações conhecidas no PR e na documentação técnica.

### 2.2 Upload da folha de respostas

Criar `POST /academia/gabarito/:id/digitalizacao`, aceitando `multipart/form-data` com `codigo_estudante` e o arquivo da folha de respostas (imagem ou PDF), reaproveitando as mesmas validações de tamanho/tipo de arquivo já usadas para documentos do sistema.

### 2.3 Extração e revisão obrigatória antes da nota

O backend deve:

1. armazenar o arquivo digitalizado de forma rastreável (mesma interface `storage.StorageProvider` já usada no restante do sistema);
2. executar o reconhecimento e extrair as respostas marcadas por questão;
3. **não** criar a `Nota` diretamente neste passo; em vez disso, retornar um resultado de correção pendente de confirmação, contendo as respostas extraídas, o número de acertos calculado e a nota resultante na escala aplicável, para que a academia possa revisar antes de confirmar;
4. permitir que a academia corrija manualmente qualquer questão que o reconhecimento tenha lido incorretamente, antes de confirmar;
5. só criar a `Nota` (com `gabarito_id` preenchido, exatamente como na Fase 1) depois de uma confirmação explícita da academia, por meio de um endpoint de confirmação (ex.: `POST /academia/gabarito/:id/digitalizacao/:correcao_id/confirmar`).

### 2.4 Testes obrigatórios

1. upload de folha de respostas válida: retorna resultado de correção pendente de confirmação, sem criar nota;
2. confirmação do resultado pendente: cria a `Nota` com `gabarito_id` preenchido;
3. correção manual de uma questão extraída incorretamente, antes da confirmação: nota final reflete a correção manual, não o valor originalmente extraído;
4. upload de arquivo inválido (não suportado pelo mecanismo de reconhecimento, corrompido, ou de tipo não aceito): rejeitado antes de qualquer tentativa de extração;
5. tentativa de confirmar um resultado de correção já confirmado anteriormente: rejeitado.

---

# 3. Atualização obrigatória da documentação

Ao final de cada fase, atualizar `Documentação.md` com:

- a entidade `Gabarito`, seus campos e validações;
- os endpoints de criação, consulta, correção estruturada (Fase 1) e digitalização/confirmação (Fase 2);
- o novo campo `gabarito_id` em `NotaDTO`;
- a decisão técnica de reconhecimento óptico adotada na Fase 2 e suas limitações conhecidas.

---

# Fora de escopo

- Correção automática de questões dissertativas/abertas; o escopo é restrito a provas objetivas de múltipla escolha.
- Criar nota automaticamente a partir da digitalização sem confirmação humana.
- Integrar esta funcionalidade com o mecanismo de reprovação por falta (tarefa 09) ou com qualquer outra tarefa deste índice além do lançamento de notas já existente.
- Suporte a formatos de folha de resposta não padronizados sem definição prévia de layout pela academia.

# Critérios de aceite

## Fase 1

1. `Gabarito` existir com os campos e validações da seção 1.1;
2. `POST /academia/gabarito` validar matéria, categoria e completude das respostas corretas;
3. `POST /academia/gabarito/:id/correcao` calcular a nota corretamente a partir de respostas estruturadas e criar a `Nota` com `gabarito_id`;
4. `NotaDTO` expor `gabarito_id` de forma opcional e retrocompatível com notas já existentes;
5. testes automatizados cobrirem os cenários da seção 1.5;
6. `Documentação.md` atualizada com a Fase 1.

## Fase 2

7. a avaliação técnica de reconhecimento óptico estar documentada com a decisão adotada;
8. `POST /academia/gabarito/:id/digitalizacao` extrair respostas sem criar nota diretamente;
9. a academia conseguir corrigir manualmente uma questão extraída incorretamente antes da confirmação;
10. a nota só ser criada após confirmação explícita;
11. testes automatizados cobrirem os cenários da seção 2.4;
12. `Documentação.md` atualizada com a Fase 2.

## Procedimento de conclusão

Ao finalizar cada fase, atualizar o título interno e o front matter apenas quando **ambas** as fases estiverem concluídas:

1. atualizar o título interno desta tarefa para `# Implementar gabarito de prova com digitalização e correção automática (feito)`;
2. alterar o front matter para `status: feito`;
3. mover este arquivo para `docs/Tarefas feitas/`.

Se apenas a Fase 1 for concluída num primeiro momento, registrar esse estado intermediário explicitamente no front matter (ex.: `status: fase_1_concluida`) em vez de marcar a tarefa inteira como concluída.
