# Dinamização de parâmetros configuráveis pela academia

## Objetivo

Permitir que cada academia configure regras acadêmicas próprias sem exigir alteração de código, mantendo limites seguros definidos pela plataforma e preservando auditoria por Event Sourcing.

## Parâmetros candidatos

### Avaliação e aprovação

- Nota mínima para aprovação por tipo de ensino, curso, ano acadêmico, período e matéria.
- Categorias de nota incluídas no cálculo da média.
- Peso de cada categoria.
- Fórmula da média: média aritmética, média ponderada, maior nota, média por trimestre + exame, recuperação.
- Obrigatoriedade de categorias antes de aprovar.
- Percentual máximo de faltas permitido.
- Regra de arredondamento: truncar, arredondar normal, arredondar para cima, casas decimais.
- Override manual: quem pode usar e quais justificativas são obrigatórias.

### Progressão e turmas

- Para qual turma/nível estudante aprovado ou reprovado deve ir.
- Estratégia de distribuição: manter turno, manter professor, balancear lotação, priorizar turma com menor lotação, escolha manual.
- Quantidade máxima de estudantes por turma.
- Permitir ou não múltiplas turmas por estudante conforme tipo de ensino.
- Permitir turma sem curso para determinados níveis.

### Registros acadêmicos

- Períodos permitidos para notas e faltas.
- Datas de abertura/fechamento de lançamento por período.
- Documentos obrigatórios na matrícula por ano/curso.
- Exigência de BI do estudante, cédula, BI do responsável, telefone do responsável.
- Campos obrigatórios por idade ou nível.

### Operação institucional

- Calendário de feriados e dias sem aula para validar faltas.
- Horários/turnos permitidos.
- Políticas de inativação de curso, matéria e turma.
- Limite de solicitações de matrícula pendentes por estudante/documento.

## Modelo de configuração proposto

Criar aggregate `AcademiaConfiguracao` ou estender `Academia` com eventos versionados, por exemplo:

- `ParametrosAcademicosDefinidos`
- `RegraAprovacaoDefinida`
- `RegraTurmaDefinida`
- `CalendarioAcademicoDefinido`
- `DocumentosMatriculaConfigurados`

A projeção pode ser `projection_academia_configuracoes` com campos:

- `codigo_academia`
- `escopo`: `global`, `fundamental`, `medio`, `superior`, `curso`, `ano_academico`, `materia`
- `configuracao` JSONB
- `version`
- `updated_at`
- `updated_by`

## Princípio de precedência

A regra mais específica vence:

1. Matéria
2. Curso + ano acadêmico
3. Curso
4. Tipo de ensino
5. Academia global
6. Default da plataforma

Exemplo: se a academia define nota mínima global 10, mas o curso de Medicina define 14, usa 14 para Medicina.

## Validações de configuração

- Pesos devem somar 100% quando a fórmula exigir ponderação.
- Categorias usadas na fórmula precisam existir e estar ativas em `projection_categorias_nota`.
- Categorias precisam estar habilitadas para os anos acadêmicos afetados.
- Nota mínima não pode ser negativa.
- Lotação máxima deve ser inteiro positivo.
- Fórmulas devem ser de um conjunto seguro pré-definido; evitar executar expressões livres arbitrárias.
- Datas de lançamento devem estar dentro do ano letivo oficial.
- Configuração não pode referenciar curso, matéria ou turma de outra academia.

## Fluxo operacional

1. Academia abre tela de configurações.
2. Backend retorna configuração efetiva: defaults da plataforma + overrides da academia.
3. Academia altera parâmetros permitidos.
4. Backend valida coerência e emite evento.
5. Projeção atualiza configuração efetiva.
6. Registros de nota/falta/avaliação passam a consultar configuração efetiva.

## Integração com avaliação final

O cálculo de aprovação deve usar um serviço de domínio como `AvaliacaoPolicy`:

1. Carregar notas do estudante no ano letivo.
2. Carregar regras efetivas da academia.
3. Verificar categorias obrigatórias.
4. Calcular média conforme fórmula configurada.
5. Comparar com nota mínima.
6. Verificar faltas máximas, se configurado.
7. Retornar decisão: aprovado, reprovado, pendente por notas/faltas ausentes ou precisa override.

## Cuidados com Event Sourcing

- A configuração usada no momento da avaliação deve ser reproduzível em rebuild.
- Opção recomendada: gravar no evento de avaliação um snapshot resumido da regra aplicada (`policy_version`, fórmula, nota mínima, categorias/pesos).
- Alterar configuração futura não deve recalcular automaticamente avaliações antigas, salvo processo explícito de reprocessamento.

## Testes recomendados

- Configurar média ponderada e aprovar com notas suficientes.
- Configurar categoria obrigatória ausente e bloquear aprovação.
- Alterar nota mínima depois de uma avaliação e garantir que histórico antigo não muda.
- Tentar configurar categoria inexistente: deve rejeitar.
- Tentar lotação máxima zero: deve rejeitar.
- Turma lotada não deve receber estudante aprovado.
