# Estudante matriculado em mais de uma instituição e solicitação de matrícula

## Objetivo

Definir como o sistema deve tratar estudantes que desejam matricular-se em mais de uma academia, considerando a funcionalidade de solicitação de matrícula.

## Estado atual observado

A solicitação de matrícula é criada para uma academia específica (`codigo_academia`). Ao aprovar a solicitação, o backend cria um novo estudante vinculado à academia. Se a solicitação possui BI principal do estudante, a aprovação verifica se já existe estudante com o mesmo BI e bloqueia duplicidade. Existe índice único para BI principal em `projection_estudantes` quando preenchido.

Isso significa que, no modelo atual, o mesmo estudante não consegue ser aprovado em outra instituição usando o mesmo BI, porque isso criaria um segundo estudante duplicado.

## Decisão de produto necessária

Há duas opções possíveis:

### Opção A — Um estudante pertence a apenas uma academia por vez

Regra simples e compatível com o modelo atual.

- BI único bloqueia duplicidade.
- Nova matrícula em outra academia deve ser tratada como transferência.
- A solicitação de matrícula de estudante já existente deve virar “solicitação de transferência/vínculo”, não criação de novo estudante.

### Opção B — Um estudante pode ter múltiplos vínculos institucionais

Mais flexível, mas exige mudança estrutural.

- Separar identidade civil do estudante (`Pessoa/Estudante`) de vínculo acadêmico (`MatriculaInstitucional`).
- BI único identifica a pessoa.
- Cada academia cria um vínculo com curso, ano, status, turmas e registros próprios.
- Notas/faltas/avaliações passam a referenciar vínculo/matrícula institucional, não apenas `codigo_estudante` global.

## Recomendação

Implementar gradualmente a Opção B se o produto realmente precisa permitir múltiplas instituições. Caso contrário, formalizar a Opção A e criar fluxo de transferência.

## Como deve funcionar com solicitação de matrícula na Opção B

1. Solicitante envia matrícula para academia X com BI do estudante.
2. Backend busca estudante existente por BI.
3. Se não existe, cria nova identidade de estudante e vínculo com academia X.
4. Se existe, não cria outro estudante; cria uma solicitação de novo vínculo institucional.
5. Academia X analisa documentos.
6. Ao aprovar, sistema cria `MatriculaInstitucionalCriada` vinculada ao estudante existente.
7. O estudante passa a visualizar múltiplas academias no perfil.
8. Ao registrar notas/faltas, academia X só acessa o vínculo dela.

## Regras de negócio para múltiplos vínculos

- BI do estudante continua único globalmente.
- `codigo_estudante` pode continuar global, mas registros acadêmicos devem incluir `codigo_academia` e idealmente `matricula_id`.
- Um estudante pode ter no máximo um vínculo ativo por academia.
- Um estudante pode ter vínculos ativos em academias diferentes, se permitido pela plataforma.
- Turmas são sempre escopadas por academia.
- Notas/faltas/avaliações são escopadas por vínculo/academia/ano letivo.
- Login do estudante deve permitir selecionar contexto da academia ao visualizar dados.

## Ajustes técnicos necessários

- Criar aggregate `MatriculaInstitucional` ou `VinculoAcademiaEstudante`.
- Migrar campos acadêmicos variáveis do estudante para o vínculo: `codigo_academia`, `ano_escolar`, `curso_medio_id`, `curso_superior_id`, status escolar, turmas atuais.
- Atualizar queries para filtrar por vínculo.
- Atualizar autorização da academia para checar vínculo ativo, não posse direta no estudante.
- Atualizar solicitação de matrícula para detectar BI existente e criar solicitação de vínculo.

## Fluxo se mantiver Opção A

1. Solicitação com BI já existente é bloqueada com mensagem “estudante já matriculado”.
2. Resposta deve orientar abrir solicitação de transferência.
3. Transferência deve encerrar vínculo anterior antes de criar novo vínculo.
4. Histórico acadêmico antigo permanece na academia de origem.

## Testes recomendados

- Solicitação para academia A cria estudante novo.
- Solicitação para academia B com mesmo BI:
  - Opção A: deve bloquear com mensagem de transferência.
  - Opção B: deve criar solicitação de vínculo sem duplicar identidade.
- Academia B não consegue ver notas da academia A.
- Estudante vê dados separados por academia.
- BI duplicado nunca cria duas identidades civis.
