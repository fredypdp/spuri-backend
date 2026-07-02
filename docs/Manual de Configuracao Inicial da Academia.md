---
criado: 02-07-2026
---

# Manual de Configuração Inicial da Academia no Spuri

Este manual orienta a academia, passo a passo, sobre tudo o que deve ser configurado para começar a usar a plataforma Spuri de forma organizada e sem bloqueios operacionais.

A ordem abaixo é importante porque algumas áreas dependem de outras. Por exemplo: não é possível registrar notas corretamente sem antes existir ano letivo, anos académicos, cursos, matérias, turmas e estudantes vinculados.

## 1. Antes de começar: entender o que precisa estar pronto

Antes de usar a plataforma no dia a dia, a academia deve preparar estes elementos:

1. Dados institucionais da academia.
2. Ano letivo ativo.
3. Estrutura académica: anos, semestres ou períodos usados pela instituição.
4. Cursos, quando aplicável.
5. Matérias ou disciplinas.
6. Categorias de nota, se a academia usar avaliações além das categorias padrão.
7. Turmas.
8. Estudantes e vínculos académicos.
9. Regras de avaliação final.
10. Rotina de lançamento de notas, faltas e encerramento do ano letivo.

> Dica: trate esta configuração como a montagem de uma escola em papel. Primeiro define-se o calendário, depois os cursos, depois as disciplinas, depois as turmas, depois os estudantes e só então começam os lançamentos de notas e faltas.

## 2. Confirmar os dados da academia

### O que é

São os dados básicos que identificam a instituição dentro da plataforma, como nome, email, tipo de instituição, nível de ensino e estrutura oferecida.

### Por que configurar primeiro

Essas informações determinam quais opções estarão disponíveis depois. Uma instituição escolar trabalha com anos académicos; uma instituição superior trabalha com semestres/períodos.

### O que verificar

A academia deve confirmar:

- Nome da instituição.
- Email institucional usado para acesso e recuperação de conta.
- Tipo de instituição: pública ou privada.
- Nível da academia:
  - escola;
  - superior.
- Para escolas, o nível escolar oferecido:
  - fundamental;
  - médio;
  - misto, quando a instituição atende fundamental e médio.
- Situação da conta: ativa.

### Como decidir

- Escolha **fundamental** se a instituição trabalha apenas do 1.º ao 9.º ano.
- Escolha **médio** se trabalha apenas com cursos médios.
- Escolha **misto** se trabalha com fundamental e médio ao mesmo tempo.
- Escolha **superior** se trabalha com cursos universitários, por semestres.

## 3. Definir o ano letivo ativo

### O que é

O ano letivo é o período oficial em que a academia está registrando aulas, notas, faltas, turmas e avaliações. O formato usado é `AAAA_AAAA`, por exemplo `2025_2026`.

### Por que vem antes de notas e faltas

Sem ano letivo ativo, a plataforma bloqueia registros académicos. Isso evita que notas ou faltas sejam lançadas no período errado.

### O que configurar

A academia deve definir o ano letivo ativo de acordo com o ano letivo global previamente aberto pelo sistema.

Exemplo:

- Ano letivo global do sistema: `2025_2026`.
- Ano letivo da academia: `2025_2026`.

### Atenção

Depois de definido, o ano letivo não deve ser alterado manualmente como se fosse uma simples edição. Ao final do período, a academia deve fazer o encerramento do ano letivo, e a plataforma avançará para o próximo período conforme a regra do sistema.

## 4. Configurar os anos académicos, semestres ou períodos

### O que é

É a estrutura que informa em quais etapas os estudantes podem estar matriculados.

Dependendo da instituição, a configuração muda:

- Ensino fundamental: anos como `1_ano`, `2_ano`, ..., `9_ano`.
- Ensino médio: anos como `1_ano`, `2_ano`, `3_ano`.
- Ensino superior: períodos/semestres como `1_semestre`, `2_semestre`, `3_semestre`, e assim por diante.

### Por que vem antes de cursos e matérias

Cursos, matérias e estudantes precisam saber a qual ano ou semestre pertencem. Se a academia ainda não configurou essa estrutura, não haverá base para organizar as turmas e os lançamentos.

### Como configurar para escola fundamental

Cadastre os anos que a escola realmente oferece.

Exemplo:

- Uma escola completa: `1_ano` até `9_ano`.
- Uma escola que atende apenas anos finais: `7_ano`, `8_ano`, `9_ano`.

### Como configurar para ensino médio

Cadastre os anos do médio que a academia oferece.

Exemplo comum:

- `1_ano`;
- `2_ano`;
- `3_ano`.

### Como configurar para ensino superior

No superior, a organização normalmente é por semestres. A academia deve informar os períodos usados pelos cursos.

Exemplo:

- Curso com 4 anos e 8 semestres: `1_semestre` até `8_semestre`.
- Curso com 3 anos e 6 semestres: `1_semestre` até `6_semestre`.

### Cuidados importantes

- Não remova um ano ou semestre que já tenha estudantes ativos vinculados.
- Use nomes padronizados para evitar confusão na equipe.
- Configure apenas etapas que a instituição realmente usa.

## 5. Cadastrar cursos

### O que é

Curso é a formação em que o estudante está matriculado. Ele é obrigatório para ensino médio profissional/técnico e para ensino superior.

Exemplos:

- Médio: Informática, Gestão Empresarial, Ciências Físicas e Biológicas.
- Superior: Engenharia Informática, Direito, Contabilidade.

### Quando é necessário

- Ensino fundamental: normalmente não precisa de curso.
- Ensino médio: precisa de curso quando há áreas/formações diferentes.
- Ensino superior: sempre precisa de curso.

### O que configurar em um curso médio

Para cada curso médio, informe:

- Nome do curso.
- Tipo: médio.
- Anos académicos do curso, por exemplo `1_ano`, `2_ano`, `3_ano`.
- Matérias-chave por ano académico.

### O que são matérias-chave

Matérias-chave são as disciplinas principais usadas para avaliar a conclusão ou progressão do estudante no curso médio. Elas devem existir para cada ano do curso.

Exemplo:

- Curso: Informática.
- `1_ano`: Matemática, Introdução à Informática.
- `2_ano`: Programação, Redes.
- `3_ano`: Projeto Tecnológico, Base de Dados.

### O que configurar em um curso superior

Para cada curso superior, informe:

- Nome do curso.
- Tipo: superior.
- Anos académicos ou duração em anos.
- Períodos/semestres do curso, por exemplo `1_semestre` até `8_semestre`.

### Cuidados importantes

- Em cursos superiores, os períodos são obrigatórios.
- Em cursos médios, as matérias-chave são obrigatórias por ano académico.
- Não crie cursos duplicados com nomes diferentes para representar a mesma formação.

## 6. Cadastrar matérias e disciplinas

### O que é

Matérias ou disciplinas são os componentes curriculares em que serão registradas notas, faltas e avaliações.

Exemplos:

- Matemática.
- Língua Portuguesa.
- Física.
- Programação.
- Direito Constitucional.

### Por que vem depois dos cursos

Algumas matérias dependem do curso. Isso acontece principalmente no ensino superior e em cursos médios específicos.

### Tipos de matéria

A matéria pode ser:

- Fundamental: usada em anos do ensino fundamental.
- Médio: usada em cursos ou anos do ensino médio.
- Superior: usada em cursos superiores e associada a um semestre/período.

### O que configurar em uma matéria fundamental

Informe:

- Nome da matéria.
- Tipo: fundamental.
- Ano ou anos académicos em que a matéria existe.

Exemplo:

- Matemática: `1_ano` até `9_ano`.
- Ciências: `5_ano` até `9_ano`.

### O que configurar em uma matéria média

Informe:

- Nome da matéria.
- Tipo: médio.
- Ano académico em que a matéria é lecionada.
- Curso relacionado, se a matéria for específica de um curso.
- Se permite pendência, quando a instituição permite que o aluno avance devendo aquela matéria.
- Se a pendência bloqueia a conclusão do nível.

### O que configurar em uma matéria superior

Informe:

- Nome da disciplina.
- Tipo: superior.
- Curso ao qual pertence.
- Período ou semestre.
- Se permite pendência.
- Se a pendência bloqueia a conclusão do curso.

### Como decidir sobre pendência

Use **pendência permitida** quando a regra pedagógica da instituição permite que o estudante avance mesmo devendo aquela disciplina.

Use **pendência que bloqueia conclusão** quando a disciplina pode ficar pendente durante o curso, mas precisa estar aprovada antes de concluir o nível.

Exemplo:

- Uma disciplina complementar pode permitir pendência e não bloquear imediatamente a progressão.
- Uma disciplina essencial do último ano pode permitir pendência, mas bloquear a conclusão se não for regularizada.

## 7. Configurar categorias de nota

### O que é

Categoria de nota é o tipo de avaliação lançado para o estudante.

A plataforma já trabalha com categorias padrão:

- Escolar: `nota_escola` e `nota_professor`.
- Superior: `nota_pp1`, `nota_pp2` e `nota_exame`.

A academia pode criar categorias adicionais quando usa avaliações próprias.

Exemplos:

- `trabalho_pratico`.
- `projeto`.
- `participacao`.
- `recuperacao`.

### Quando configurar

Configure antes de começar a lançar notas. Assim, os professores e a secretaria já terão as opções corretas disponíveis.

### O que informar

Para cada categoria adicional, informe:

- Código curto, sem espaços, por exemplo `projeto_final`.
- Nome visível, por exemplo Projeto Final.
- Descrição, se necessário.
- Anos académicos em que essa categoria pode ser usada.

### Cuidados importantes

- Não crie categorias demais sem necessidade.
- Use nomes claros para a equipa entender quando usar cada uma.
- Evite alterar a lógica de avaliação no meio do ano letivo sem comunicar a equipe.

## 8. Criar turmas

### O que é

Turma é o grupo em que os estudantes ficam organizados para aulas e acompanhamento.

Exemplos:

- `7A` — 7.º ano, turma A.
- `INF1A` — Informática, 1.º ano, turma A.
- `DIR3N` — Direito, 3.º ano ou semestre, período noturno.

### Por que vem depois de anos, cursos e matérias

A turma precisa representar uma etapa real da academia. Para isso, a plataforma precisa saber quais anos, cursos e períodos já existem.

### O que configurar

Para cada turma, informe:

- Código da turma.
- Nome ou descrição da turma.
- Ano académico ou semestre relacionado.
- Curso, quando aplicável.
- Turno:
  - manhã;
  - tarde;
  - noite.
- Gênero, se a instituição separar turmas por gênero.

### Boas práticas para código de turma

Use um padrão simples e fácil de reconhecer.

Exemplos:

- Fundamental: `1A`, `2B`, `9A`.
- Médio: `INF1A`, `INF2A`, `GES3B`.
- Superior: `DIR1M`, `DIR2N`, `ENG5N`.

Evite códigos longos, repetidos ou difíceis de lembrar.

## 9. Cadastrar estudantes e seus vínculos

### O que é

É o cadastro do aluno e a ligação dele com a academia, ano, curso e turma.

### Por que vem depois das turmas

O estudante precisa ser colocado em uma estrutura já existente. Se a turma ou o curso ainda não existe, o cadastro fica incompleto ou incorreto.

### O que cadastrar

Para cada estudante, informe:

- Nome completo.
- Documento de identificação, quando disponível.
- Data de nascimento.
- Email ou telefone, se a academia usar esses dados.
- Nível de ensino.
- Ano académico ou semestre.
- Curso, quando aplicável.
- Turma.
- Status escolar.

### Opções de status

- Ativo: estudante atualmente matriculado e acompanhado.
- Inativo: estudante sem vínculo ativo naquele momento.
- Arquivado: estudante mantido no histórico, mas fora da operação corrente.

### Matrícula por solicitação

Se a academia usa solicitações de matrícula, o fluxo recomendado é:

1. Receber a solicitação.
2. Conferir os dados e documentos.
3. Aprovar ou reprovar a solicitação.
4. Após aprovação, vincular o estudante à estrutura correta.

### Cuidados importantes

- Confira o nome e documento antes de salvar.
- Evite cadastrar o mesmo estudante duas vezes.
- Confira se o estudante está no ano, curso e turma corretos antes de iniciar lançamentos.

## 10. Configurar regras de avaliação final

### O que é

A regra de avaliação final define como a plataforma deve decidir se um estudante foi aprovado, reprovado, promovido, finalizado ou ficou com pendências.

### Por que configurar antes do fechamento

Sem regra clara, a equipe pode lançar notas e faltas, mas não terá uma decisão final automática e padronizada.

### O que definir

A academia deve definir:

- Nível ao qual a regra se aplica: fundamental, médio ou superior.
- Ano académico, curso ou semestre afetado, quando a regra tiver escopo específico.
- Fórmula ou critério de cálculo.
- Nota mínima para aprovação.
- Tratamento de matérias pendentes.
- Matérias que participam da decisão final.
- Situações que bloqueiam conclusão.

### Exemplos didáticos

#### Fundamental

Um estudante do fundamental pode ser aprovado quando atinge a média mínima nas matérias exigidas e não ultrapassa o limite de faltas.

#### Médio

Um estudante do médio pode avançar para o próximo ano se cumprir a média mínima, mas algumas matérias podem gerar pendência conforme a regra da instituição.

#### Superior

Um estudante do superior normalmente é avaliado por disciplina e semestre. A aprovação pode depender de notas como PP1, PP2 e exame, conforme a fórmula adotada.

### Cuidados importantes

- Configure a regra antes do período de fechamento.
- Valide a regra com a direção pedagógica.
- Não use uma regra genérica se cursos diferentes têm exigências diferentes.
- Documente internamente qualquer exceção.

## 11. Iniciar lançamentos de notas

### O que é

É o registro das avaliações dos estudantes nas matérias configuradas.

### Pré-requisitos

Antes de lançar notas, confirme que existem:

- Ano letivo ativo.
- Estudante ativo.
- Turma correta.
- Matéria correta.
- Categoria de nota correta.
- Período correto.

### Como lançar corretamente

Para cada nota, selecione:

- Estudante.
- Matéria/disciplina.
- Período, por exemplo trimestre ou semestre.
- Categoria da nota.
- Valor da nota.
- Ano letivo.

### Períodos comuns

Para escolar:

- `1_trimestre`;
- `2_trimestre`;
- `3_trimestre`.

Para superior:

- `1_semestre`;
- `2_semestre`;
- demais semestres configurados no curso.

### Cuidados importantes

- Não lance nota em matéria errada apenas para “guardar depois”.
- Confira se a categoria corresponde ao tipo de ensino.
- Evite duplicidade: a mesma nota não deve ser registrada duas vezes para o mesmo estudante, matéria, período, categoria e ano letivo.

## 12. Iniciar lançamentos de faltas

### O que é

É o registro de ausências dos estudantes nas aulas ou atividades.

### Pré-requisitos

Antes de lançar faltas, confirme que existem:

- Ano letivo ativo.
- Estudante ativo.
- Matéria/disciplina correta.
- Data correta da falta.

### Como lançar corretamente

Para cada falta, selecione:

- Estudante.
- Matéria/disciplina.
- Data da falta.
- Ano letivo.
- Observação, se necessário.

### Cuidados importantes

- Registre a falta na data real.
- Evite duplicar a mesma falta para o mesmo estudante, matéria, data e ano letivo.
- Use observações apenas quando forem úteis para a secretaria ou coordenação.

## 13. Acompanhar registros e corrigir inconsistências cedo

### O que acompanhar

Durante o ano letivo, a academia deve acompanhar:

- Estudantes sem turma.
- Estudantes em curso ou ano errado.
- Matérias sem notas quando já deveriam ter avaliação.
- Faltas em excesso.
- Categorias de nota usadas incorretamente.
- Turmas desatualizadas.

### Por que fazer isso durante o ano

Corrigir problemas no início é mais simples do que tentar corrigir tudo no fechamento. A plataforma depende da estrutura configurada para calcular e apresentar dados corretamente.

## 14. Realizar avaliação final

### O que é

A avaliação final consolida o desempenho do estudante no ano letivo ou período e registra a decisão final.

### Pré-requisitos

Antes de executar a avaliação final, confirme:

- Todas as notas foram lançadas.
- Todas as faltas relevantes foram registradas.
- As matérias-chave estão configuradas, quando aplicável.
- As regras de avaliação final estão corretas.
- O estudante está vinculado ao curso, ano e turma corretos.

### Possíveis resultados

Dependendo do nível e da regra, o estudante pode ficar como:

- Aprovado ou promovido.
- Reprovado.
- Finalizado, quando conclui o nível ou curso.
- Com pendência, quando a regra da academia permite.

### Cuidados importantes

- Não execute avaliação final antes de conferir notas e faltas.
- Evite reavaliar sem necessidade.
- Guarde internamente a data de fechamento e os responsáveis pela conferência.

## 15. Encerrar o ano letivo

### O que é

É a declaração de que a academia terminou oficialmente o ano letivo ativo.

### Quando fazer

Faça somente depois de:

1. Concluir lançamentos de notas.
2. Concluir lançamentos de faltas.
3. Executar avaliações finais necessárias.
4. Conferir estudantes aprovados, reprovados, finalizados e pendentes.
5. Resolver inconsistências administrativas.

### O que acontece depois

Ao finalizar o ano letivo, a plataforma registra o encerramento e avança a academia para o ano letivo seguinte, conforme as regras do sistema.

### Cuidados importantes

- Não finalize o ano letivo se ainda existem notas ou faltas pendentes.
- Não finalize apenas para “testar”.
- Combine o encerramento com a direção ou secretaria académica.

## 16. Ordem cronológica resumida

Use esta lista como checklist de implantação:

1. Confirmar dados e status da academia.
2. Definir ano letivo ativo.
3. Configurar anos académicos, semestres ou períodos.
4. Cadastrar cursos, quando aplicável.
5. Cadastrar matérias e disciplinas.
6. Configurar matérias-chave dos cursos médios.
7. Criar categorias adicionais de nota, se necessário.
8. Criar turmas.
9. Cadastrar ou aprovar estudantes.
10. Vincular estudantes a ano, curso e turma.
11. Configurar regras de avaliação final.
12. Lançar notas.
13. Lançar faltas.
14. Acompanhar relatórios e corrigir inconsistências.
15. Executar avaliações finais.
16. Finalizar ano letivo.

## 17. Checklist de validação antes de liberar a academia para uso

A academia está pronta para operar quando responder “sim” para todos os itens abaixo:

- A conta da academia está ativa?
- O email institucional está correto?
- O ano letivo ativo foi definido?
- Os anos académicos ou semestres foram configurados?
- Os cursos necessários foram cadastrados?
- As matérias foram cadastradas e ligadas aos anos, cursos ou semestres corretos?
- As matérias-chave dos cursos médios foram configuradas?
- As categorias extras de nota foram cadastradas, se a academia precisar delas?
- As turmas foram criadas com códigos claros?
- Os estudantes foram cadastrados sem duplicidade?
- Os estudantes estão vinculados ao ano, curso e turma corretos?
- As regras de avaliação final foram definidas e revisadas pela direção pedagógica?
- A equipe sabe quando lançar notas e faltas?
- A equipe sabe que o ano letivo só deve ser finalizado após a conferência geral?

## 18. Recomendações para a equipe da academia

- Defina um responsável principal pela configuração inicial.
- Evite que muitas pessoas alterem configurações estruturais ao mesmo tempo.
- Use padrões simples para nomes de turmas, cursos e categorias.
- Faça uma conferência antes de começar a lançar notas reais.
- Sempre corrija a estrutura primeiro; depois corrija os lançamentos.
- Mantenha uma rotina semanal de verificação de notas, faltas e estudantes sem vínculo correto.

## 19. Glossário rápido

- **Academia**: instituição de ensino que usa a plataforma.
- **Ano letivo**: período oficial de funcionamento académico, como `2025_2026`.
- **Ano académico**: etapa do estudante, como 1.º ano, 2.º ano ou 9.º ano.
- **Período/Semestre**: etapa usada principalmente no ensino superior.
- **Curso**: formação em que o estudante está matriculado.
- **Matéria/Disciplina**: componente curricular que recebe notas e faltas.
- **Categoria de nota**: tipo de avaliação, como nota do professor, exame ou projeto.
- **Turma**: grupo de estudantes organizado por ano, curso, turno ou sala.
- **Pendência**: matéria ou disciplina que o estudante ainda precisa regularizar.
- **Avaliação final**: processo que decide a situação final do estudante no período.
- **Finalização do ano letivo**: encerramento oficial do ano pela academia.
