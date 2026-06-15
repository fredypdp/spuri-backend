# Dinamização de parâmetros por academia

## Objetivo
Permitir que cada academia configure regras operacionais e pedagógicas próprias dentro de limites definidos pela plataforma, reduzindo regras fixas no código e facilitando adaptação a escolas, universidades e modelos mistos.

## Princípio geral
A plataforma deve ter padrões seguros globais. A academia pode sobrescrever parâmetros quando autorizado, mas toda alteração deve gerar evento auditável e manter histórico por ano letivo.

## Escopo de configurações dinâmicas

### Avaliação e aprovação
- Nota mínima para aprovação por nível, curso, ano académico, matéria ou período.
- Fórmula de média: média aritmética, ponderada, maior nota, recuperação substitutiva, exame final ponderado.
- Categorias de notas incluídas no cálculo: prova, trabalho, nota professor, exame, avaliação contínua.
- Pesos por categoria de nota.
- Arredondamento: casas decimais, arredondar para cima, para baixo ou matemático.
- Recuperação: se existe, nota mínima para ir à recuperação e fórmula após recuperação.
- Reprovação automática por faltas, se aplicável.
- Aprovação por conselho pedagógico, com justificativa obrigatória.

### Progressão acadêmica
- Para qual ano/turma estudantes aprovados avançam.
- Para qual turma estudantes reprovados permanecem.
- Critérios de turma destino: mesma letra, mesmo turno, turma com menor ocupação, turma definida manualmente.
- Permitir ou não estudante sem turma temporária após aprovação.
- Regras de conclusão de ciclo.
- Regras de equivalência/transferência externa.

### Turmas
- Quantidade máxima de estudantes por turma.
- Política ao atingir lotação: bloquear, permitir lista de espera ou exigir autorização.
- Critérios de distribuição por gênero, turno, curso ou ano académico.
- Permitir estudantes de anos diferentes na mesma turma, em casos especiais.
- Regras de alteração de curso/nível com estudantes vinculados.

### Faltas
- Limite máximo de faltas por matéria, período ou ano letivo.
- Se faltas contam por quantidade simples ou por carga horária.
- Se justificativas abonam faltas ou apenas registram observação.
- Prazo máximo para registrar/editar falta retroativa.
- Bloqueio de faltas fora do período oficial do ano letivo.

### Matrícula e cadastro
- Documentos obrigatórios por nível e ano académico.
- Obrigatoriedade de telefone, email, B.I. do estudante, B.I. do responsável e telefone do responsável.
- Idade mínima/máxima por ano académico.
- Permitir matrícula em mais de uma instituição e sob quais condições.
- Regras para estudante transferido de outra academia.

### Calendário e períodos
- Períodos avaliativos usados pela academia: trimestres, semestres, módulos, bimestres.
- Datas internas de início/fim de cada período dentro do ano letivo global.
- Datas limite para lançamento de notas, faltas e avaliação final.
- Período de matrícula/rematrícula.

### Segurança e auditoria
- Perfis da academia autorizados a alterar configurações.
- Exigir justificativa para mudanças sensíveis.
- Exigir confirmação dupla para mudanças que afetam cálculo de média já existente.
- Bloquear mudanças retroativas após encerramento do período/ano letivo, exceto por fluxo administrativo auditado.

## Hierarquia de regras
1. Regra global da plataforma, definida por Admin FPP.
2. Regra por nível de ensino da academia.
3. Regra por curso.
4. Regra por ano académico.
5. Regra por matéria.
6. Regra por turma.

A regra mais específica prevalece, desde que não viole limites globais.

## Modelo de configuração sugerido
Criar agregado de configuração acadêmica versionado por academia e ano letivo:
- `codigo_academia`.
- `ano_letivo`.
- `escopo`: `academia`, `curso`, `ano_academico`, `materia`, `turma`.
- `escopo_id`, opcional.
- `tipo_configuracao`.
- `valor_json`.
- `vigente_de`, `vigente_ate`.
- `status`: `rascunho`, `ativo`, `substituido`, `revogado`.
- `alterado_por`, `alterado_em`, `justificativa`.

## Fluxo operacional
1. Academia acessa painel de configurações.
2. Sistema carrega padrões globais e configurações atuais.
3. Academia altera parâmetros permitidos.
4. Sistema valida coerência e limites globais.
5. Sistema mostra simulação do impacto, principalmente para médias e lotação.
6. Academia confirma com justificativa quando necessário.
7. Sistema grava evento de configuração alterada.
8. Novos registros usam a versão ativa da configuração.
9. Registros antigos continuam apontando para a versão da regra usada no momento do cálculo.

## Validações
- Pesos de média ponderada devem somar 100% ou 1.0, conforme padrão escolhido.
- Nota mínima deve estar dentro da escala de notas da academia.
- Categorias usadas na fórmula devem existir e estar ativas.
- Fórmula não pode referenciar categorias inexistentes.
- Lotação máxima deve ser número positivo.
- Regras por turma não podem conflitar com nível/curso da turma.
- Mudanças em ano letivo finalizado devem ser bloqueadas.
- Mudanças retroativas devem criar nova versão e recalcular apenas mediante comando explícito.

## Critérios de aceite
- Academia consegue configurar nota mínima, fórmula de média, categorias envolvidas e lotação máxima.
- Sistema mantém padrões quando a academia não define configurações próprias.
- Alterações são auditáveis e versionadas.
- Cálculos de notas indicam qual versão de regra foi utilizada.
- Regras globais do Admin FPP limitam configurações inválidas.
